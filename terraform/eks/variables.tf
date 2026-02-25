variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string
}

variable "region" {
  description = "AWS region for the EKS cluster"
  type        = string
  default     = "us-east-1"
}

variable "kubernetes_version" {
  description = "Kubernetes version for the EKS cluster"
  type        = string
  default     = "1.32"
}

variable "node_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 3
}

variable "instance_type" {
  description = "EC2 instance type for worker nodes (e.g., m5.large for AMD, m7g.large for ARM/Graviton)"
  type        = string
  default     = "m5.large"
}

variable "node_arch" {
  description = "Node architecture: amd64 or arm64"
  type        = string
  default     = "amd64"

  validation {
    condition     = contains(["amd64", "arm64"], var.node_arch)
    error_message = "node_arch must be either 'amd64' or 'arm64'"
  }
}
