output "instance_id" {
  value = aws_instance.runner.id
}

output "primary_network_interface_id" {
  value = aws_instance.runner.primary_network_interface_id
}

output "previous_network_interface_id" {
  value = aws_network_interface.previous_owner.id
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

output "ssm_output_prefix" {
  value = "s3://${aws_s3_bucket.artifacts.bucket}/${local.ssm_output_prefix}"
}
