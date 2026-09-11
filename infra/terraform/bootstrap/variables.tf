variable "aws_region" {
  description = "AWS region everything in this deployment lives in."
  type        = string
  default     = "ap-northeast-1"
}

variable "project" {
  description = "Short name used as a prefix for every resource this config creates."
  type        = string
  default     = "guardpipe"
}
