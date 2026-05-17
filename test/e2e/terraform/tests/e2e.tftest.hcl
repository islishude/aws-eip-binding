run "aws_e2e" {
  command = apply

  assert {
    condition     = aws_ssm_association.e2e.association_id != ""
    error_message = "SSM association did not complete."
  }

  assert {
    condition     = aws_ssm_association.e2e.output_location[0].s3_bucket_name == aws_s3_bucket.artifacts.bucket
    error_message = "SSM association output is not configured for the artifact bucket."
  }

  assert {
    condition     = aws_network_interface.previous_owner.id != aws_instance.runner.primary_network_interface_id
    error_message = "Previous-owner ENI must be distinct from the runner primary ENI."
  }
}
