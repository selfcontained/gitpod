variable "kubeconfig" {
    description = "Path to the KUBECONFIG file to connect to the cluster"
    default = "./kubeconfig"
}

variable "cert_manager_issuer" {
    default     = null
}

variable "secretAccessKey" {
    default     = null
}

variable "issuer_name" {
    default     = "route53"
}
