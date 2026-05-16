resource "aws_s3_bucket" "artifacts" {
  bucket        = local.bucket_name
  force_destroy = true

  tags = {
    Name = "${var.name_prefix}-artifacts"
  }
}

resource "aws_s3_bucket_public_access_block" "artifacts" {
  bucket                  = aws_s3_bucket.artifacts.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_object" "binary" {
  bucket       = aws_s3_bucket.artifacts.id
  key          = local.artifact_key
  source       = var.binary_path
  etag         = filemd5(var.binary_path)
  content_type = "application/octet-stream"

  depends_on = [
    aws_s3_bucket_public_access_block.artifacts,
    aws_s3_bucket_server_side_encryption_configuration.artifacts,
  ]
}
