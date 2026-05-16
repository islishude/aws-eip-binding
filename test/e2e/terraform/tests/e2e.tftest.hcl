run "aws_e2e" {
  command = apply

  assert {
    condition     = aws_ssm_association.e2e.association_id != ""
    error_message = "SSM association did not complete."
  }
}
