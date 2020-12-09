Feature: Ansible Integration
  Scenario: Ad-hoc commands
    When I SSH to EC2 Instance and execute Ansible ad-hoc
    Then I get a 0 response back for success