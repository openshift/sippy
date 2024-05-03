# Examples

## Example Input file with a list of TestIds to match
```
{
    "Arguments": {
        "TestRelease": "4.16",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-05T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-03-29T00:00:00Z&component=Networking%20%2F%20cluster-network-operator&capability=Other&environment=ovn%20amd64%20metal-ipi&network=ovn&arch=amd64&platform=metal-ipi",
        "IssueDescription": "CNO PODS",
        "IssueType": "Product",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "test_cno_pods_output.json",
        "OutputType": "JSON",
        "IncidentGroupId": "f2dda7d3-b504-4a4f-b342-3d225a26d3e7",
        "TargetModifiedTime": "2024-04-05 20:40:48.517998+00:00",
        "JobRunStartTimeMax": "2024-04-01T00:58:50Z",
        "JobRunStartTimeMin": "2024-03-31T03:24:33Z"
    },
    "Tests": [
        { "TestId": "openshift-tests-upgrade:cc1518431a9dbf5c839f29edc86a51c0"},
        { "TestId": "openshift-tests-upgrade:07a835d0c2d8e48df6a12d8e0206d67a"},
        { "TestId": "openshift-tests-upgrade:44e2e0e6106443fef746afb65a3aaa9f"},
        { "TestId": "openshift-tests-upgrade:fbe6ebd6d5f577a21de3de9504ca242a"},
        { "TestId": "openshift-tests-upgrade:a7bca0ce3787e8bd213b32795d81bb50"},
        { "TestId": "openshift-tests-upgrade:c2f88e80fa2064a98711768d5a679735"}        
    ]
}
```

```
./gen-resolved-issue.py --input-file=test_cno_pods_input.json
```

Review `test_cno_pods_output.json`, remove the OutputType or change it to DB

Run the command to persist the entries to DB
```
./gen-resolved-issue.py --input-file=test_cno_pods_output.json --output-type=DB --load-incidents-from-file
```


## Example Pulling in Test Information Only
Start with a wildcard for the TestId and a minimal list of Variants
```
{
    "Arguments": {
        "TestRelease": "4.15",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2023-10-31T23:59:59Z&baseRelease=4.14&baseStartTime=2023-10-04T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-30T23:59:59Z&sampleRelease=4.15&sampleStartTime=2024-04-23T00:00:00Z",
        "IssueDescription": "CLEAR EVERYTHING",
        "IssueType": "Infrastructure",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "metal_4_15_regressions.json",
        "OutputType": "JSON"
    },
    "Tests": [
        {
            "Release": "4.15",
            "TestId": "*",
            "Variants": [
                             {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
               
        }
    ]
}
```

Run the command and specify output-test-info-only=True
This will skip adding job runs and allow the output to be used for a different release and / or time period
```
./gen-resolved-issue.py --input-file=metal_4_15_input.json --output-test-info-only=true
```

The output file can be edited and renamed (`metal_4_16_input.json `) to change the TestRelease to 4.16, update the TestReportURL and the OutputFile name.  Then rerun the command specifying the 4.16 input file and omitting the output-test-info-only flag to create the full 4.16 specific output that matches only the provided tests and variants.

```
{
    "Arguments": {
        "TestRelease": "4.16",
        "TestReportURL": "https://sippy.dptools.openshift.org/api/component_readiness?baseEndTime=2024-02-28T23:59:59Z&baseRelease=4.15&baseStartTime=2024-02-01T00:00:00Z&confidence=95&excludeArches=arm64,heterogeneous,ppc64le,s390x&excludeClouds=openstack,ibmcloud,libvirt,ovirt,unknown&excludeVariants=hypershift,osd,microshift,techpreview,single-node,assisted,compact&groupBy=cloud,arch,network&ignoreDisruption=true&ignoreMissing=false&minFail=3&pity=5&sampleEndTime=2024-04-30T23:59:59Z&sampleRelease=4.16&sampleStartTime=2024-04-23T00:00:00Z",
        "IssueDescription": "CLEAR EVERYTHING",
        "IssueType": "Infrastructure",
        "IssueURL": "https://issues.redhat.com/browse/TRT-1555",
        "OutputFile": "metal_4_16_regressions.json",
        "OutputType": "JSON",
        "IncidentGroupId": "5adf9bf4-75e8-493e-a71e-d3455d3c9f1c",
        "TargetModifiedTime": "2024-04-30 17:49:14.654868+00:00"
    },
    "Tests": [
        {
            "TestId": "Operator results:868edf33fa32b0aab132c2a676d49e3b",
            "TestName": "operator conditions cluster-autoscaler",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:ff3f4ce2ada4b853ece12306b1ef3eaf",
            "TestName": "operator conditions machine-api",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:45d55df296fbbfa7144600dce70c1182",
            "TestName": "operator conditions etcd",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:ad47fd0f8db4a5195cee022678627c9b",
            "TestName": "operator conditions image-registry",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "cluster install:0cb1bb27e418491b1ffdacab58c5c8c0",
            "TestName": "install should succeed: overall",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "cluster install:2bc0fe9de9a98831c20e569a21d7ded9",
            "TestName": "install should succeed: cluster creation",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:e78644c3024c99c0a7226427e95fb8e9",
            "TestName": "[sig-arch] events should not repeat pathologically for ns/openshift-console",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:258e3ff8c9692c937596663377c10e29",
            "TestName": "operator conditions console",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:7e4c8db94dde9f957ea7d639cd29d6dd",
            "TestName": "operator conditions monitoring",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:b3997eeabb330f3000872f22d6ddb618",
            "TestName": "[bz-networking][invariant] alert/OVNKubernetesResourceRetryFailure should not be at or above info",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:4b5f6af893ad5577904fbaec3254506d",
            "TestName": "operator conditions network",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:33921465a4b24f992f7e9c47b1ec9409",
            "TestName": "operator conditions ingress",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:55a75a8aa11231d0ca36a4d65644e1dd",
            "TestName": "operator conditions operator-lifecycle-manager-packageserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:776d244e9df7ada04b8510480fb86902",
            "TestName": "operator conditions openshift-samples",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:2bc3a57ebccf0bcb4d36d338809848c2",
            "TestName": "operator conditions kube-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:2bc3a57ebccf0bcb4d36d338809848c2",
            "TestName": "operator conditions kube-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "openshift-tests:b4f339155fefdaea15a98fc78a8b9177",
            "TestName": "[sig-arch] events should not repeat pathologically for ns/openshift-kube-controller-manager",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:50009b9589c6c5db8d438d0a551a4681",
            "TestName": "operator conditions kube-scheduler",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:466e1a49a33b63218495dc8201953194",
            "TestName": "operator conditions authentication",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        },
        {
            "TestId": "Operator results:a4dfe6caa55e94230b4ee0ff127b6d64",
            "TestName": "operator conditions openshift-apiserver",
            "Variants": [
                {
                    "key": "Platform",
                    "value": "metal-ipi"
                }
            ]
        }
    ]
}
```