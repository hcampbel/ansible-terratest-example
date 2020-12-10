terraform {
  # This module is now only being tested with Terraform 0.13.x. However, to make upgrading easier, we are setting
  # 0.12.26 as the minimum version, as that version added support for required_providers with source URLs, making it
  # forwards compatible with 0.13.x code.
  required_version = ">= 0.12.26"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 3.0"
    }
  }
}

provider "aws" {
  region                  = var.aws_region
  shared_credentials_file = var.creds
}

resource "aws_instance" "public_node" {
  ami                    = data.aws_ami.centos.id
  instance_type          = "t2.micro"
  vpc_security_group_ids = [aws_security_group.test_sg.id]
  key_name               = var.key_pair_name
  subnet_id              = var.subnet

  # This EC2 Instance has a public IP and will be accessible directly from the public Internet
  associate_public_ip_address = true

  tags = {
    Name = "${var.instance_name}-public"
  }
}

resource "aws_instance" "private_node" {
  ami                    = data.aws_ami.centos.id
  instance_type          = "t2.micro"
  vpc_security_group_ids = [aws_security_group.test_sg.id]
  key_name               = var.key_pair_name
  subnet_id              = var.subnet

  # This EC2 Instance has a private IP and will be accessible only from within the VPC
  associate_public_ip_address = false

  tags = {
    Name = "${var.instance_name}-private"
  }
}

resource "aws_security_group" test_sg {
  name = var.instance_name

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port = var.ssh_port
    to_port   = var.ssh_port
    protocol  = "tcp"

    # To keep this example simple, we allow incoming SSH requests from any IP. In real-world usage, you should only
    # allow SSH requests from trusted servers, such as a bastion host or VPN server.
    cidr_blocks = ["0.0.0.0/0"]
  }
}


# ---------------------------------------------------------------------------------------------------------------------
# LOOK UP THE LATEST CENTOS AMI WITH ANSIBLE 2.9 INSTALLED
# ---------------------------------------------------------------------------------------------------------------------

data "aws_ami" "centos" {
  most_recent = true
  owners      = ["978816831872"] # CentOS

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  filter {
    name   = "image-type"
    values = ["machine"]
  }

  filter {
    name   = "name"
    values = ["CentOS-8.3-Ansible-2.9"]
  }
}

