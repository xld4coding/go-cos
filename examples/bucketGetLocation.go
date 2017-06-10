package main

import (
	"bitbucket.org/mozillazg/go-cos"
	"context"
	"fmt"
	"os"
	"time"
)

func main() {
	b, _ := cos.ParseBucketFromDomain("test-1253846586.cn-north.myqcloud.com")
	c := cos.NewClient(os.Getenv("COS_SECRETID"), os.Getenv("COS_SECRETKEY"), b, nil)
	startTime := time.Now()
	endTime := startTime.Add(time.Hour)
	acl, _, err := c.Bucket.GetLocation(context.Background(), cos.NewAuthTime(
		startTime, endTime,
		startTime, endTime))
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%#v", acl)
}