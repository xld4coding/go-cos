package cos

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestBucketService_GetACL(t *testing.T) {
	setup()
	defer teardown()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		vs := values{
			"acl": "",
		}
		testFormValues(t, r, vs)
		fmt.Fprint(w, `<AccessControlPolicy>
	<Owner>
		<uin>100000760461</uin>
	</Owner>
	<AccessControlList>
		<Grant>
			<Grantee type="RootAccount">
				<uin>100000760461</uin>
			</Grantee>
			<Permission>FULL_CONTROL</Permission>
		</Grant>
		<Grant>
			<Grantee type="RootAccount">
				<uin>100000760463</uin>
			</Grantee>
			<Permission>READ</Permission>
		</Grant>
	</AccessControlList>
</AccessControlPolicy>`)
	})

	ref, _, err := client.Bucket.GetACL(context.Background(), NewAuthTime(time.Minute))
	if err != nil {
		t.Fatalf("Bucket.GetACL returned error: %v", err)
	}

	want := &BucketGetACLResult{
		XMLName: xml.Name{Local: "AccessControlPolicy"},
		Owner: &Owner{
			UIN: "100000760461",
		},
		AccessControlList: []ACLGrant{
			{
				Grantee: &ACLGrantee{
					Type: "RootAccount",
					UIN:  "100000760461",
				},
				Permission: "FULL_CONTROL",
			},
			{
				Grantee: &ACLGrantee{
					Type: "RootAccount",
					UIN:  "100000760463",
				},
				Permission: "READ",
			},
		},
	}

	if !reflect.DeepEqual(ref, want) {
		t.Errorf("Bucket.GetACL returned %+v, want %+v", ref, want)
	}

}

func TestBucketService_PutACL_with_header_opt(t *testing.T) {
	setup()
	defer teardown()

	opt := &BucketPutACLOptions{
		Header: &ACLHeaderOptions{
			XCosACL: "private",
		},
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		testMethod(t, r, http.MethodPut)
		vs := values{
			"acl": "",
		}
		testFormValues(t, r, vs)
		testHeader(t, r, "x-cos-acl", "private")

		want := http.NoBody
		v := r.Body
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Bucket.PutACL request body: %#v, want %#v", v, want)
		}
	})

	_, err := client.Bucket.PutACL(context.Background(), NewAuthTime(time.Minute), opt)
	if err != nil {
		t.Fatalf("Bucket.PutACL returned error: %v", err)
	}

}

func TestBucketService_PutACL_with_body_opt(t *testing.T) {
	setup()
	defer teardown()

	opt := &BucketPutACLOptions{
		Body: &ACLXml{
			Owner: &Owner{
				UIN: "100000760461",
			},
			AccessControlList: []ACLGrant{
				{
					Grantee: &ACLGrantee{
						Type: "RootAccount",
						UIN:  "100000760461",
					},

					Permission: "FULL_CONTROL",
				},
				{
					Grantee: &ACLGrantee{
						Type: "RootAccount",
						UIN:  "100000760463",
					},
					Permission: "READ",
				},
			},
		},
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		v := new(ACLXml)
		xml.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, http.MethodPut)
		vs := values{
			"acl": "",
		}
		testFormValues(t, r, vs)
		testHeader(t, r, "x-cos-acl", "")

		want := opt.Body
		want.XMLName = xml.Name{Local: "AccessControlPolicy"}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Bucket.PutACL request body: %+v, want %+v", v, want)
		}

	})

	_, err := client.Bucket.PutACL(context.Background(), NewAuthTime(time.Minute), opt)
	if err != nil {
		t.Fatalf("Bucket.PutACL returned error: %v", err)
	}

}
