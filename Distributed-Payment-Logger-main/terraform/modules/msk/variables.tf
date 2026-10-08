variable "project_name" { type = string }
variable "enable_msk" { type = bool }
variable "msk_cluster_name" { type = string }
variable "vpc_id" { type = string }
variable "private_subnet_ids" { type = list(string) }
variable "security_group_ids" { type = list(string) }
