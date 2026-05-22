# HCL (Terraform): searching the resource type returns the whole block.
terraform {
  required_version = ">= 1.5"
}

resource "aws_s3_bucket" "billing_archive" {
  bucket = "billing-archive"

  versioning {
    enabled = true
  }

  tags = {
    Name = "billing-archive"
  }
}
