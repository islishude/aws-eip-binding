resource "aws_ssm_association" "e2e" {
  name             = "AWS-RunShellScript"
  association_name = "${var.name_prefix}-e2e"

  targets {
    key    = "InstanceIds"
    values = [aws_instance.runner.id]
  }

  parameters = {
    commands = local.e2e_script
  }

  wait_for_success_timeout_seconds = var.ssm_timeout_seconds

  depends_on = [
    aws_iam_role_policy.runner,
    aws_iam_role_policy_attachment.ssm,
    aws_s3_object.binary,
    aws_vpc_endpoint.ssm,
  ]
}
