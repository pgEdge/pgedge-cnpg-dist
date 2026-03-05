variable "cluster_name" {
  description = "Name of the EKS cluster"
  type        = string

  validation {
    condition     = can(regex("^[0-9A-Za-z][A-Za-z0-9_-]*$", var.cluster_name)) && length(var.cluster_name) <= 100
    error_message = "cluster_name must be 1-100 characters, start with an alphanumeric, and contain only letters, digits, hyphens, or underscores."
  }
}

variable "region" {
  description = "AWS region for the EKS cluster"
  type        = string
  default     = "ap-south-1"
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

  validation {
    condition     = var.node_count > 0
    error_message = "node_count must be a positive integer."
  }
}

variable "instance_type" {
  description = "EC2 instance type for worker nodes (e.g., m5.large for AMD, m7g.large for ARM/Graviton)"
  type        = string
  default     = "m5.large"
}

variable "eks_api_allowed_cidrs" {
  description = "List of CIDRs allowed to reach the EKS public API endpoint. Defaults to unrestricted; override with your org/CI runner CIDRs for tighter control (e.g., [\"203.0.113.0/24\"])."
  type        = list(string)
  default     = ["0.0.0.0/0"]
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
