package prowloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/apis/prow"
	"github.com/openshift/sippy/pkg/prowloader/gcs"
	"google.golang.org/api/iterator"
	
)

func TestProwToGCS(t *testing.T) {



	// if testsDisabled {
	// 	return
	// }

	b, err := ioutil.ReadFile("./prowjob_test.json")

	if err != nil {
		t.Fatalf("Error reading file: %#v", err.Error())
	}

	//pj prow.ProwJob
	pj := &prow.ProwJob{}
	err = json.Unmarshal(b, pj)
	if err != nil {
		t.Fatalf("cannot decode JSON: %v", err)
	}

	pjURL, err := url.Parse(pj.Status.URL)

	

	if err != nil {
		t.Fatalf("Error parsing URL: %v", err)
	}

	parts := strings.Split(pjURL.Path, /*pl.bktName*/ "origin-ci-test")
	if len(parts) == 2 {
		fmt.Printf("Part 1: %s, Part 2: %s\n", parts[0], parts[1])
	} else {
		t.Fatalf("Invalid number of URL parts: %d", len(parts))
	}

	if(len(pj.Spec.Refs.Pulls) < 1) {
		t.Fatal("Invalid Spec Ref Pulls")
	}

	// part[1] up ../..?
	// or pj.Spec.Refs.Pulls[0].Number?
	prId := fmt.Sprint(pj.Spec.Refs.Pulls[0].Number)
	parts = strings.Split(parts[1], prId)

	if len(parts) == 2 {
		fmt.Printf("Part 1: %s, Part 2: %s\n", parts[0], parts[1])
	} else {
		t.Fatalf("Invalid number of PR root parts: %d", len(parts))
	}
	
	prRoot := fmt.Sprintf("%s%s/", parts[0], prId)

	prRoot = strings.TrimPrefix(prRoot, "/")

	getPRRootJobs(prRoot)

}

func getPRRootJobs(prRoot string) (string, error){

	url := fmt.Sprintf("https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/%s", prRoot)

	fmt.Println(url)

	gcsClient, err := gcs.NewGCSClient(context.TODO(),
	 /*o.GoogleServiceAccountCredentialFile*/"/home/fsb/gcp/openshift-ci-data-analysis-c10194eec5e2.json",
	 /*.GoogleOAuthClientCredentialFile*/"",
	)

	if(err != nil) {
		return "", err
	}

	bkt := gcsClient.Bucket("origin-ci-test")

	// handle.

	bkt.Object(prRoot)
	it := bkt.Objects(context.Background(), &storage.Query{
		Prefix: prRoot,
		Delimiter: "/",
	})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}

		// want empty Name
		// then get the prefix latest-build.txt
		// from that append the value to the prefix
		// and look for finished.json

		if len(attrs.Name) > 0 {
			continue
		}

		if !evaluateJobStatus(bkt, attrs.Prefix) {
			fmt.Printf("Job: %s is not finished\n", attrs.Prefix)
		}
	}

	return "", nil
}

func evaluateJobStatus(bkt *storage.BucketHandle, prefix string) bool{

		//bkt.Object(prefix + "finished.json")

		//
		jobRun := gcs.NewGCSJobRun(bkt, "" )
		bytes, err := jobRun.GetContent(context.TODO(), fmt.Sprintf("%s%s", prefix, "latest-build.txt"))
		if err != nil {
			fmt.Printf("Error %v", err)
		}

		latest := string(bytes)
		latestPath := fmt.Sprintf("%s%s/finishedXX.json", prefix, latest)

		return jobRun.ContentExists(context.TODO(), latestPath)


// //		jobRun = gcs.NewGCSJobRun(bkt,  latestPath)
// 		bytes, err = jobRun.GetContent(context.TODO(), latestPath)
// 		if err != nil {
// 			fmt.Printf("Error %v", err)
// 		}

// 		var x map[string]interface{}

// 		json.Unmarshal(bytes, &x)

// 		fmt.Printf("%v", x)
}