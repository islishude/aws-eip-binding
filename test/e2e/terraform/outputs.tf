output "instance_id" {
  value = aws_instance.runner.id
}

output "primary_network_interface_id" {
  value = aws_instance.runner.primary_network_interface_id
}

output "target_ipv4" {
  value = aws_eip.target.public_ip
}

output "target_ipv6" {
  value = local.target_ipv6
}

output "ssm_association_id" {
  value = aws_ssm_association.e2e.association_id
}
