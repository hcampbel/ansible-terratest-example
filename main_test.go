package ansible_terratest_example

import (
	"fmt"
	"io/ioutil"
	_ "log"
	"os"
	_ "os"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/aws"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/gruntwork-io/terratest/modules/ssh"
	"github.com/gruntwork-io/terratest/modules/terraform"
	cmdShell "github.com/gruntwork-io/terratest/modules/shell"
	testStructure "github.com/gruntwork-io/terratest/modules/test-structure"
)

const (
	awsRegion	= "us-east-1"

)

var rootFolder string
var terraformFolder string

func TestTerraformSshExample(t *testing.T) {
	t.Parallel()

	rootFolder = "."
	terraformFolder = "./terraform"


	exampleFolder := testStructure.CopyTerraformFolderToTemp(t, rootFolder, terraformFolder)

	// At the end of the test, run `terraform destroy` to clean up any resources that were created
	defer testStructure.RunTestStage(t, "teardown", func() {
		terraformOptions := testStructure.LoadTerraformOptions(t, exampleFolder)
		terraform.Destroy(t, terraformOptions)

		keyPair := testStructure.LoadEc2KeyPair(t, exampleFolder)
		aws.DeleteEC2KeyPair(t, keyPair)
	})

	// Deploy the example
	testStructure.RunTestStage(t, "setup", func() {
		terraformOptions, keyPair := configureTerraformOptions(t, exampleFolder)

		// Save the options and key pair so later test stages can use them
		testStructure.SaveTerraformOptions(t, exampleFolder, terraformOptions)
		testStructure.SaveEc2KeyPair(t, exampleFolder, keyPair)

		// This will run `terraform init` and `terraform apply` and fail the test if there are any errors
		terraform.InitAndApply(t, terraformOptions)
	})

	// Make sure we can SSH to the public Instance directly from the public Internet and the private Instance by using
	// the public Instance as a jump host
	testStructure.RunTestStage(t, "validate", func() {
		terraformOptions := testStructure.LoadTerraformOptions(t, exampleFolder)
		keyPair := testStructure.LoadEc2KeyPair(t, exampleFolder)

		testSSHToPublicHost(t, terraformOptions, keyPair)
		testSSHAgentToPublicHost(t, terraformOptions, keyPair)
		testAnsibleAgainstPublicHost(t, terraformOptions, keyPair)
	})

}


func configureTerraformOptions(t *testing.T, exampleFolder string) (*terraform.Options, *aws.Ec2Keypair) {
	// A unique ID we can use to namespace resources so we don't clash with anything already in the AWS account or
	// tests running in parallel
	uniqueID := random.UniqueId()

	// Give this EC2 Instance and other resources in the Terraform code a name with a unique ID so it doesn't clash
	// with anything else in the AWS account.
	instanceName := fmt.Sprintf("terratest-example-%s", uniqueID)

	// Pick a random AWS region to test in. This helps ensure your code works in all regions.
	// awsRegion := aws.GetRandomStableRegion(t, nil, nil)

	// Create an EC2 KeyPair that we can use for SSH access
	keyPairName := fmt.Sprintf("terratest-example-%s", uniqueID)
	keyPair := aws.CreateAndImportEC2KeyPair(t, awsRegion, keyPairName)


	// Construct the terraform options with default retryable errors to handle the most common retryable errors in
	// terraform testing.
	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		// The path to where our Terraform code is located
		TerraformDir: exampleFolder,

		// Variables to pass to our Terraform code using -var options
		Vars: map[string]interface{}{
			"aws_region":    awsRegion,
			"instance_name": instanceName,
			"key_pair_name": keyPairName,
		},
	})

	return terraformOptions, keyPair
}

func testSSHAgentToPublicHost(t *testing.T, terraformOptions *terraform.Options, keyPair *aws.Ec2Keypair) {
	// Run `terraform output` to get the value of an output variable
	publicInstanceIP := terraform.Output(t, terraformOptions, "public_instance_ip")

	// start the ssh agent
	sshAgent := ssh.SshAgentWithKeyPair(t, keyPair.KeyPair)
	defer sshAgent.Stop()

	// We're going to try to SSH to the instance IP, using the Key Pair we created earlier. Instead of
	// directly using the SSH key in the SSH connection, we're going to rely on an existing SSH agent that we
	// programatically emulate within this test. We're going to use the user "centos" as we know the Instance
	// is running an centos AMI that has such a user
	publicHost := ssh.Host{
		Hostname:         publicInstanceIP,
		SshUserName:      "centos",
		OverrideSshAgent: sshAgent,
	}

	// It can take a minute or so for the Instance to boot up, so retry a few times
	maxRetries := 30
	timeBetweenRetries := 15 * time.Second
	description := fmt.Sprintf("SSH with Agent to public host %s", publicInstanceIP)

	// Run a simple echo command on the server
	expectedText := "Hello, World"
	command := fmt.Sprintf("echo -n '%s'", expectedText)

	// Verify that we can SSH to the Instance and run commands
	retry.DoWithRetry(t, description, maxRetries, timeBetweenRetries, func() (string, error) {

		actualText, err := ssh.CheckSshCommandE(t, publicHost, command)

		if err != nil {
			return "", err
		}

		if strings.TrimSpace(actualText) != expectedText {
			return "", fmt.Errorf("Expected SSH command to return '%s' but got '%s'", expectedText, actualText)
		}

		return "", nil
	})
}

func testSSHToPublicHost(t *testing.T, terraformOptions *terraform.Options, keyPair *aws.Ec2Keypair) {
	// Run `terraform output` to get the value of an output variable
	publicInstanceIP := terraform.Output(t, terraformOptions, "public_instance_ip")

	// We're going to try to SSH to the instance IP, using the Key Pair we created earlier, and the user "centos",
	// as we know the Instance is running an centos AMI that has such a user
	publicHost := ssh.Host{
		Hostname:    publicInstanceIP,
		SshKeyPair:  keyPair.KeyPair,
		SshUserName: "centos",
	}

	// It can take a minute or so for the Instance to boot up, so retry a few times
	maxRetries := 30
	timeBetweenRetries := 15 * time.Second
	description := fmt.Sprintf("SSH to public host %s", publicInstanceIP)

	// Run a simple echo command on the server
	expectedText := "Hello, World"
	command := fmt.Sprintf("echo -n '%s'", expectedText)

	// Verify that we can SSH to the Instance and run commands
	retry.DoWithRetry(t, description, maxRetries, timeBetweenRetries, func() (string, error) {
		actualText, err := ssh.CheckSshCommandE(t, publicHost, command)

		if err != nil {
			return "", err
		}

		if strings.TrimSpace(actualText) != expectedText {
			return "", fmt.Errorf("Expected SSH command to return '%s' but got '%s'", expectedText, actualText)
		}

		return "", nil
	})

	// Run Ansible command against the server
	expectedText = "Hello, World"
	command = fmt.Sprintf("echo -n '%s' && exit 1", expectedText)
	description = fmt.Sprintf("SSH to public host %s with error command", publicInstanceIP)

	// Verify that we can SSH to the Instance, run the command and see the output
	retry.DoWithRetry(t, description, maxRetries, timeBetweenRetries, func() (string, error) {

		actualText, err := ssh.CheckSshCommandE(t, publicHost, command)

		if err == nil {
			return "", fmt.Errorf("Expected SSH command to return an error but got none")
		}

		if strings.TrimSpace(actualText) != expectedText {
			return "", fmt.Errorf("Expected SSH command to return '%s' but got '%s'", expectedText, actualText)
		}

		return "", nil
	})
}

func testAnsibleAgainstPublicHost(t *testing.T, terraformOptions *terraform.Options, keyPair *aws.Ec2Keypair) {
	// Run `terraform output` to get the value of an output variable
	var ansibleCmd cmdShell.Command

	publicInstanceIP := terraform.Output(t, terraformOptions, "public_instance_ip")
	// inventoryFileName := "hosts"
	successText := fmt.Sprintf("%s | SUCCESS => {\"ansible_facts\": {     \"discovered_interpreter_python\": \"/usr/libexec/platform-python\"   },   \"changed\": false,    \"ping\": \"pong\"}", publicInstanceIP)

	tmpFile, err := ioutil.TempFile(".", "hosts")

	if err != nil {
		fmt.Errorf("%s", err)
		return
	}

	defer os.Remove(tmpFile.Name())
	fmt.Println("Created File: " + tmpFile.Name())

	text := []byte("[all]\n"+publicInstanceIP)
	if _, err = tmpFile.Write(text); err != nil {
		fmt.Errorf("%s", err)
		return
	}

	if err = tmpFile.Close(); err != nil {
		fmt.Errorf("%s", err)
	}

	ansibleCmd = cmdShell.Command{Command: "ansible", Args: []string{ "-i", tmpFile.Name(), "-m", "ping", "all", "-u", "centos"}, WorkingDir: "."}

	fmt.Println("Executing Ansible Test...")
	actualText, ansibleCmdErr := cmdShell.RunCommandAndGetOutputE(t, ansibleCmd)

	if strings.TrimSpace(actualText) != successText {
		fmt.Errorf("Expected command to return '%s' but got '%s'", successText, ansibleCmdErr)
		return
	}
}