package main

import (
	"context"
	"net/url"
	"os"
	"time"

	"bitbucket.org/mozillazg/go-cos"
)

func main() {
	u, _ := url.Parse("https://testhuanan-1253846586.cn-south.myqcloud.com")
	b := &cos.BaseURL{
		BucketURL: u,
	}
	c := cos.NewClient(os.Getenv("COS_SECRETID"), os.Getenv("COS_SECRETKEY"), b, nil)
	c.Client.Transport = &cos.DebugRequestTransport{
		RequestHeader:  true,
		RequestBody:    true,
		ResponseHeader: true,
		ResponseBody:   true,
	}

	lc := &cos.BucketPutLifecycleOptions{
		Rules: []cos.BucketLifecycleRule{
			{
				ID:     "1234",
				Prefix: "test",
				Status: "Enabled",
				Transition: &cos.BucketLifecycleTransition{
					Days:         10,
					StorageClass: "Standard",
				},
			},
			{
				ID:     "123422",
				Prefix: "gg",
				Status: "Disabled",
				Expiration: &cos.BucketLifecycleExpiration{
					Days: 10,
				},
			},
		},
	}
	_, err := c.Bucket.PutLifecycle(context.Background(), cos.NewAuthTime(time.Hour), lc)
	if err != nil {
		panic(err)
	}
}
