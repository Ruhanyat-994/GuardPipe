# Planted true-positive for depscan.secrets.committed-credential.
AWS_ACCESS_KEY = "AKIAIOSFODNN7EXAMPLE"

# Near-miss — must NOT fire (placeholder value, not a real secret).
DEFAULT_PASSWORD = "changeme"

# Near-miss — must NOT fire (env var reference, not a literal).
DB_PASSWORD = os.environ["DB_PASSWORD"]
