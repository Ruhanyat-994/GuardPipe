// Package dockerx is a thin wrapper over the Docker Engine API client
// (documentation/05-module-specifications.md §1: "sandbox | M6 | 1 |
// dockerx"). It knows nothing about GuardPipe's sandbox security policy —
// that enforcement lives in adapters/sandbox, which is built on top of this
// package. dockerx only does generic container lifecycle operations: pull,
// create, start, wait, read logs, remove, list-by-label.
package dockerx

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
)

// Client wraps the real Docker Engine API client.
type Client struct {
	cli *dockerclient.Client
}

// New connects to the Docker daemon at host (GUARDPIPE_DOCKER_HOST,
// documentation/13-devops-and-environments.md §5.4) — an empty host falls
// back to the client's usual environment-variable resolution (DOCKER_HOST,
// or the platform default). Like repo.New/queue.New, this only prepares the
// client; it does not verify the daemon is actually reachable.
func New(host string) (*Client, error) {
	opts := []dockerclient.Opt{dockerclient.FromEnv}
	if host != "" {
		opts = append(opts, dockerclient.WithHost(host))
	}
	cli, err := dockerclient.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("dockerx: create client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close releases the client's resources.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping verifies the Docker daemon is actually reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx, dockerclient.PingOptions{})
	return err
}

// PullImage pulls ref, draining (and discarding) the progress stream the
// API returns — callers that need pull progress don't exist yet, and this
// is a thin wrapper, not a progress-reporting layer.
func (c *Client) PullImage(ctx context.Context, ref string) error {
	rc, err := c.cli.ImagePull(ctx, ref, dockerclient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("dockerx: pull image %s: %w", ref, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("dockerx: read pull progress for %s: %w", ref, err)
	}
	return nil
}

// CreateContainer creates a container from config/hostConfig — both are
// Docker's own wire types, passed through unmodified. adapters/sandbox owns
// what goes into them; this method only calls the API.
func (c *Client) CreateContainer(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, name string) (string, error) {
	result, err := c.cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostConfig,
		Name:       name,
	})
	if err != nil {
		return "", fmt.Errorf("dockerx: create container: %w", err)
	}
	return result.ID, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	if _, err := c.cli.ContainerStart(ctx, id, dockerclient.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("dockerx: start container %s: %w", id, err)
	}
	return nil
}

// WaitContainer blocks until the container stops running, ctx is
// cancelled/deadlined, or an error arrives — whichever comes first. A
// cancelled/deadlined ctx is how the caller enforces RunSpec.Timeout
// (documentation/04-backend-architecture.md §7.1); this method itself has
// no timeout logic of its own.
func (c *Client) WaitContainer(ctx context.Context, id string) (exitCode int64, err error) {
	result := c.cli.ContainerWait(ctx, id, dockerclient.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-result.Error:
		return 0, fmt.Errorf("dockerx: wait for container %s: %w", id, err)
	case res := <-result.Result:
		return res.StatusCode, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Logs returns a container's stdout and stderr, demultiplexed from
// Docker's single combined stream (RunResult.Stdout/.Stderr are separate
// per documentation/04-backend-architecture.md §7.1).
func (c *Client) Logs(ctx context.Context, id string) (stdout, stderr []byte, err error) {
	rc, err := c.cli.ContainerLogs(ctx, id, dockerclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, nil, fmt.Errorf("dockerx: read logs for %s: %w", id, err)
	}
	defer rc.Close()

	stdout, stderr, err = demuxLogs(rc)
	if err != nil {
		return nil, nil, fmt.Errorf("dockerx: demultiplex logs for %s: %w", id, err)
	}
	return stdout, stderr, nil
}

// RemoveContainer force-removes a container and its anonymous volumes.
// Called from a defer by every caller (adapters/sandbox) so a crashed or
// timed-out run never leaks a container.
func (c *Client) RemoveContainer(ctx context.Context, id string) error {
	_, err := c.cli.ContainerRemove(ctx, id, dockerclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		return fmt.Errorf("dockerx: remove container %s: %w", id, err)
	}
	return nil
}

// ListContainerIDsByLabel returns every container (running or not) carrying
// label=value — how adapters/sandbox's startup orphan sweep finds
// containers a previous crashed process never cleaned up
// (documentation/04-backend-architecture.md §7.2: "Removal: force remove in
// a defer, plus an orphan sweep at startup").
func (c *Client) ListContainerIDsByLabel(ctx context.Context, label, value string) ([]string, error) {
	result, err := c.cli.ContainerList(ctx, dockerclient.ContainerListOptions{
		All:     true,
		Filters: dockerclient.Filters{}.Add("label", label+"="+value),
	})
	if err != nil {
		return nil, fmt.Errorf("dockerx: list containers by label: %w", err)
	}
	ids := make([]string, len(result.Items))
	for i, item := range result.Items {
		ids[i] = item.ID
	}
	return ids, nil
}

// demuxLogs parses Docker's stdcopy frame format (used whenever a
// container's Config.Tty is false): each frame is an 8-byte header
// — [stream type, 0, 0, 0, size(4 bytes big-endian)] — followed by that
// many bytes of payload. Stream type 1 is stdout, 2 is stderr; anything
// else (0 = stdin) is skipped. Hand-rolled rather than importing
// docker/docker/pkg/stdcopy: github.com/moby/moby/client is a standalone
// module split out of the moby/moby monorepo and doesn't carry pkg/stdcopy
// with it, and the format itself is a stable, tiny, and well-documented
// part of the Engine API — not worth a second module dependency for.
func demuxLogs(r io.Reader) (stdout, stderr []byte, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	header := make([]byte, 8)

	for {
		if _, err := io.ReadFull(r, header); err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, nil, err
		}
		switch header[0] {
		case 1:
			stdoutBuf.Write(payload)
		case 2:
			stderrBuf.Write(payload)
		}
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}
