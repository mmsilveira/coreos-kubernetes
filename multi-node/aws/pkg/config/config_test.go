package config

import (
	"net"
	"reflect"
	"testing"
)

const minimalConfigYaml = `externalDNSName: test.staging.core-os.net
keyName: test-key-name
region: us-west-1
clusterName: test-cluster-name
kmsKeyArn: "arn:aws:kms:us-west-1:xxxxxxxxx:key/xxxxxxxxxxxxxxxxxxx"
`

const availabilityZoneConfig = `
availabilityZone: us-west-1c
`

const singleAzConfigYaml = minimalConfigYaml + availabilityZoneConfig

var goodNetworkingConfigs = []string{
	``, //Tests validity of default network config values
	`
vpcCIDR: 10.4.3.0/24
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.100.101
`, `
vpcCIDR: 10.4.0.0/16
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 10.6.0.0/16
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`, `
vpcId: vpc-xxxxx
routeTableId: rtb-xxxxxx
`, `
vpcId: vpc-xxxxx
`, `
createRecordSet: false
hostedZone: ""
`, `
createRecordSet: true
recordSetTTL: 400
hostedZone: core-os.net
`, `
createRecordSet: true
hostedZone: "staging.core-os.net"
`,
}

var incorrectNetworkingConfigs = []string{
	`
vpcCIDR: 10.4.2.0/23
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 10.4.0.0/16 #podCIDR contains vpcCDIR.
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`,
	`
vpcCIDR: 10.4.2.0/23
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 10.5.0.0/16
serviceCIDR: 10.4.0.0/16 #serviceCIDR contains vpcCDIR.
dnsServiceIP: 10.4.100.101
`, `
vpcCIDR: 10.4.0.0/16
instanceCIDR: 10.5.3.0/24 #instanceCIDR not in vpcCIDR
controllerIP: 10.5.3.5
podCIDR: 10.6.0.0/16
serviceCIDR: 10.5.0.0/16
dnsServiceIP: 10.5.100.101
`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.0.1 #dnsServiceIP conflicts with kubernetesServiceIP
`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 10.4.0.0/16 #vpcCIDR overlaps with podCIDR
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.5.100.101

`, `
vpcCIDR: 10.4.3.0/16
instanceCIDR: 10.4.3.0/24
controllerIP: 10.4.3.5
podCIDR: 172.4.0.0/16
serviceCIDR: 172.5.0.0/16
dnsServiceIP: 172.6.100.101 #dnsServiceIP not in service CIDR
`, `
routeTableId: rtb-xxxxxx # routeTableId specified without vpcId
`, `
# invalid TTL
recordSetTTL: 0
`, `
# hostedZone shouldn't be blank when createRecordSet is true
createRecordSet: true
hostedZone: ""
`, `
# recordSetTTL shouldn't be modified when createRecordSet is false
createRecordSet: false
recordSetTTL: 400
`, `
# whatever.com is not a superdomain of test.staging.core-os.net
createRecordSet: true
hostedZone: "whatever.com"
`,
}

func TestNetworkValidation(t *testing.T) {

	for _, networkConfig := range goodNetworkingConfigs {
		configBody := singleAzConfigYaml + networkConfig
		if _, err := ClusterFromBytes([]byte(configBody)); err != nil {
			t.Errorf("Correct config tested invalid: %s\n%s", err, networkConfig)
		}
	}

	for _, networkConfig := range incorrectNetworkingConfigs {
		configBody := singleAzConfigYaml + networkConfig
		if _, err := ClusterFromBytes([]byte(configBody)); err == nil {
			t.Errorf("Incorrect config tested valid, expected error:\n%s", networkConfig)
		}
	}

}

func TestKubernetesServiceIPInference(t *testing.T) {

	// We sill assert that after parsing the network configuration,
	// KubernetesServiceIP is the correct pre-determined value
	testConfigs := []struct {
		NetworkConfig       string
		KubernetesServiceIP string
	}{
		{
			NetworkConfig: `
serviceCIDR: 172.5.10.10/22
dnsServiceIP: 172.5.10.10
        `,
			KubernetesServiceIP: "172.5.8.1",
		},
		{
			NetworkConfig: `
serviceCIDR: 10.5.70.10/18
dnsServiceIP: 10.5.64.10
        `,
			KubernetesServiceIP: "10.5.64.1",
		},
		{
			NetworkConfig: `
serviceCIDR: 172.4.155.98/27
dnsServiceIP: 172.4.155.100
        `,
			KubernetesServiceIP: "172.4.155.97",
		},
		{
			NetworkConfig: `
serviceCIDR: 10.6.142.100/28
dnsServiceIP: 10.6.142.100
        `,
			KubernetesServiceIP: "10.6.142.97",
		},
	}

	for _, testConfig := range testConfigs {
		configBody := singleAzConfigYaml + testConfig.NetworkConfig
		cluster, err := ClusterFromBytes([]byte(configBody))
		if err != nil {
			t.Errorf("Unexpected error parsing config: %v\n %s", err, configBody)
			continue
		}

		_, serviceNet, err := net.ParseCIDR(cluster.ServiceCIDR)
		if err != nil {
			t.Errorf("invalid serviceCIDR: %v", err)
			continue
		}

		kubernetesServiceIP := incrementIP(serviceNet.IP)
		if kubernetesServiceIP.String() != testConfig.KubernetesServiceIP {
			t.Errorf("KubernetesServiceIP mismatch: got %s, expected %s",
				kubernetesServiceIP,
				testConfig.KubernetesServiceIP)
		}
	}

}

func TestIsSubdomain(t *testing.T) {
	validData := []struct {
		sub    string
		parent string
	}{
		{
			// single level
			sub:    "test.coreos.com",
			parent: "coreos.com",
		},
		{
			// multiple levels
			sub:    "cgag.staging.coreos.com",
			parent: "coreos.com",
		},
		{
			// trailing dots shouldn't matter
			sub:    "staging.coreos.com.",
			parent: "coreos.com.",
		},
		{
			// trailing dots shouldn't matter
			sub:    "a.b.c.",
			parent: "b.c",
		},
		{
			// multiple level parent domain
			sub:    "a.b.c.staging.core-os.net",
			parent: "staging.core-os.net",
		},
	}

	invalidData := []struct {
		sub    string
		parent string
	}{
		{
			// mismatch
			sub:    "staging.coreos.com",
			parent: "example.com",
		},
		{
			// superdomain is longer than subdomain
			sub:    "staging.coreos.com",
			parent: "cgag.staging.coreos.com",
		},
	}

	for _, valid := range validData {
		if !isSubdomain(valid.sub, valid.parent) {
			t.Errorf("%s should be a valid subdomain of %s", valid.sub, valid.parent)
		}
	}

	for _, invalid := range invalidData {
		if isSubdomain(invalid.sub, invalid.parent) {
			t.Errorf("%s should not be a valid subdomain of %s", invalid.sub, invalid.parent)
		}
	}

}

func TestReleaseChannel(t *testing.T) {

	validConfigs := []struct {
		conf    string
		channel string
	}{
		{
			conf: `
releaseChannel: alpha
`,
			channel: "alpha",
		},
		{
			conf: `
releaseChannel: beta
`,
			channel: "beta",
		},
	}

	invalidConfigs := []string{
		`
#TODO(chom): move this to validConfigs when stable is supported
releaseChannel: stable # stable is not supported (yet).
`,
		`
releaseChannel: non-existant #this release channel will never exist
`,
	}

	for _, conf := range validConfigs {
		confBody := singleAzConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if c.ReleaseChannel != conf.channel {
			t.Errorf(
				"parsed release channel %s does not match config: %s",
				c.ReleaseChannel,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := singleAzConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config: %s", confBody)
		}
	}

}

func TestMultipleSubnets(t *testing.T) {

	validConfigs := []struct {
		conf    string
		subnets []Subnet
	}{
		{
			conf: `
# You can specify multiple subnets to be created in order to achieve H/A
vpcCIDR: 10.4.3.0/16
controllerIP: 10.4.3.50
subnets:
  - availabilityZone: ap-northeast-1a
    instanceCIDR: 10.4.3.0/24
  - availabilityZone: ap-northeast-1c
    instanceCIDR: 10.4.4.0/24
`,
			subnets: []Subnet{
				{
					InstanceCIDR:     "10.4.3.0/24",
					AvailabilityZone: "ap-northeast-1a",
				},
				{
					InstanceCIDR:     "10.4.4.0/24",
					AvailabilityZone: "ap-northeast-1c",
				},
			},
		},
		{
			conf: `
# Given AZ/CIDR, missing subnets fall-back to the single subnet with the AZ/CIDR given.
vpcCIDR: 10.4.3.0/16
controllerIP: 10.4.3.50
availabilityZone: ap-northeast-1a
instanceCIDR: 10.4.3.0/24
`,
			subnets: []Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.4.3.0/24",
				},
			},
		},
		{
			conf: `
# Given AZ/CIDR, empty subnets fall-back to the single subnet with the AZ/CIDR given.
vpcCIDR: 10.4.3.0/16
controllerIP: 10.4.3.50
availabilityZone: ap-northeast-1a
instanceCIDR: 10.4.3.0/24
subnets: []
`,
			subnets: []Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.4.3.0/24",
				},
			},
		},
		{
			conf: `
# Given no AZ/CIDR, empty subnets fall-backs to the single subnet with the default az/cidr.
availabilityZone: "ap-northeast-1a"
subnets: []
`,
			subnets: []Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.0.0.0/24",
				},
			},
		},
		{
			conf: `
# Missing subnets field fall-backs to the single subnet with the default az/cidr.
availabilityZone: "ap-northeast-1a"
`,
			subnets: []Subnet{
				{
					AvailabilityZone: "ap-northeast-1a",
					InstanceCIDR:     "10.0.0.0/24",
				},
			},
		},
	}

	invalidConfigs := []string{
		`
# You can't specify both the top-level availability zone and subnets
# (It doesn't make sense. Which configuration did you want, single or multi AZ one?)
availabilityZone: "ap-northeast-1a"
subnets:
  - availabilityZone: "ap-northeast-1b"
    instanceCIDR: "10.0.0.0/24"
`,
		`
# You can't specify both the top-level instanceCIDR and subnets
# (It doesn't make sense. Which configuration did you want, single or multi AZ one?)
instanceCIDR: "10.0.0.0/24"
subnets:
- availabilityZone: "ap-northeast-1b"
  instanceCIDR: "10.0.1.0/24"
`,
		`
subnets:
# Missing AZ like this
# - availabilityZone: "ap-northeast-1a"
- instanceCIDR: 10.0.0.0/24
`,
		`
subnets:
# Missing AZ like this
# - availabilityZone: "ap-northeast-1a"
- instanceCIDR: 10.0.0.0/24
`,
		`
subnets:
# Both AZ/instanceCIDR is given. This is O.K. but...
- availabilityZone: "ap-northeast-1a"
# instanceCIDR does not include the default controllerIP
- instanceCIDR: 10.0.5.0/24
`,
		`
subnets:
# Overlapping subnets
- availabilityZone: "ap-northeast-1a"
  instanceCIDR: 10.0.5.0/24
- availabilityZone: "ap-northeast-1b"
  instanceCIDR: 10.0.5.0/24
`,
	}

	for _, conf := range validConfigs {
		confBody := minimalConfigYaml + conf.conf
		c, err := ClusterFromBytes([]byte(confBody))
		if err != nil {
			t.Errorf("failed to parse config %s: %v", confBody, err)
			continue
		}
		if !reflect.DeepEqual(c.Subnets, conf.subnets) {
			t.Errorf(
				"parsed subnets %s does not expected subnets %s in config: %s",
				c.Subnets,
				conf.subnets,
				confBody,
			)
		}
	}

	for _, conf := range invalidConfigs {
		confBody := minimalConfigYaml + conf
		_, err := ClusterFromBytes([]byte(confBody))
		if err == nil {
			t.Errorf("expected error parsing invalid config: %s", confBody)
		}
	}

}
