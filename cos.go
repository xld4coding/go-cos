package cos

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"github.com/google/go-querystring/query"
	"github.com/mozillazg/go-httpheader"
)

const (
	// Version ...
	Version               = "0.13.0"
	userAgent             = "go-cos/" + Version
	contentTypeXML        = "application/xml"
	defaultServiceBaseURL = "https://service.cos.myqcloud.com"
)

var bucketURLTemplate = template.Must(
	template.New("bucketURLFormat").Parse(
		"{{.Scheme}}://{{.BucketName}}-{{.AppID}}.cos.{{.Region}}.csyun001.ccbcos.com",
	),
)

// BaseURL 访问各 API 所需的基础 URL
type BaseURL struct {
	// 访问 bucket, object 相关 API 的基础 URL（不包含 path 部分）
	// 比如：https://test-1253846586.cos.ap-beijing.myqcloud.com
	// 详见 https://cloud.tencent.com/document/product/436/6224
	BucketURL *url.URL
	// 访问 service API 的基础 URL（不包含 path 部分）
	// 比如：https://service.cos.myqcloud.com
	ServiceURL *url.URL
}

// NewBaseURL 生成 BaseURL
func NewBaseURL(bucketURL string) (u *BaseURL, err error) {
	bu, err := url.Parse(bucketURL)
	if err != nil {
		return
	}
	su, _ := url.Parse(defaultServiceBaseURL)
	u = &BaseURL{
		BucketURL:  bu,
		ServiceURL: su,
	}
	return
}

// NewBucketURL 生成 BaseURL 所需的 BucketURL
//
//   bucketName: bucket 名称
//   AppID: 应用 ID
//   Region: 区域代码，详见 https://cloud.tencent.com/document/product/436/6224
//   secure: 是否使用 https
func NewBucketURL(bucketName, appID, region string, secure bool) *url.URL {
	scheme := "https"
	if !secure {
		scheme = "http"
	}

	w := bytes.NewBuffer(nil)
	bucketURLTemplate.Execute(w, struct {
		Scheme     string
		BucketName string
		AppID      string
		Region     string
	}{
		scheme, bucketName, appID, region,
	})

	u, _ := url.Parse(w.String())
	return u
}

// A Client manages communication with the COS API.
type Client struct {
	// Sender 用于实际发送 HTTP 请求
	Sender Sender
	// ResponseParser 用于解析响应
	ResponseParser ResponseParser

	UserAgent string
	BaseURL   *BaseURL

	common service

	// Service 封装了 service 相关的 API
	Service *ServiceService
	// Bucket 封装了 bucket 相关的 API
	Bucket *BucketService
	// Object 封装了 object 相关的 API
	Object *ObjectService
}

type service struct {
	client *Client
}

// NewClient returns a new COS API client.
// 使用 DefaultSender 作为 Sender，DefaultResponseParser 作为 ResponseParser
func NewClient(uri *BaseURL, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	baseURL := &BaseURL{}
	if uri != nil {
		baseURL.BucketURL = uri.BucketURL
		baseURL.ServiceURL = uri.ServiceURL
	}
	if baseURL.ServiceURL == nil {
		baseURL.ServiceURL, _ = url.Parse(defaultServiceBaseURL)
	}

	c := &Client{
		Sender:         &DefaultSender{httpClient},
		ResponseParser: &DefaultResponseParser{},
		UserAgent:      userAgent,
		BaseURL:        baseURL,
	}
	c.common.client = c
	c.Service = (*ServiceService)(&c.common)
	c.Bucket = (*BucketService)(&c.common)
	c.Object = (*ObjectService)(&c.common)
	return c
}

func (c *Client) newRequest(ctx context.Context, opt *sendOptions) (req *http.Request, err error) {
	baseURL := opt.baseURL
	uri := opt.uri
	method := opt.method
	body := opt.body
	optQuery := opt.optQuery
	optHeader := opt.optHeader

	uri, err = addURLOptions(uri, optQuery)
	if err != nil {
		return
	}
	u, _ := url.Parse(uri)
	urlStr := baseURL.ResolveReference(u).String()

	var reader io.Reader
	contentType := ""
	contentMD5 := ""
	xsha1 := ""
	if body != nil {
		// 上传文件
		if r, ok := body.(io.Reader); ok {
			reader = r
		} else {
			b, err := xml.Marshal(body)
			if err != nil {
				return nil, err
			}
			contentType = contentTypeXML
			reader = bytes.NewReader(b)
			contentMD5 = base64.StdEncoding.EncodeToString(calMD5Digest(b))
			// xsha1 = base64.StdEncoding.EncodeToString(calSHA1Digest(b))
		}
	} else {
		contentType = contentTypeXML
	}

	req, err = http.NewRequest(method, urlStr, reader)
	if err != nil {
		return
	}

	req.Header, err = addHeaderOptions(req.Header, optHeader)
	if err != nil {
		return
	}
	if v := req.Header.Get("Content-Length"); req.ContentLength == 0 && v != "" && v != "0" {
		req.ContentLength, _ = strconv.ParseInt(v, 10, 64)
		req.Body = ioutil.NopCloser(reader)
	}

	if contentMD5 != "" {
		req.Header["Content-MD5"] = []string{contentMD5}
	}
	if xsha1 != "" {
		req.Header.Set("x-cos-sha1", xsha1)
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if req.Header.Get("Content-Type") == "" && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return
}

func (c *Client) doAPI(ctx context.Context, caller Caller, req *http.Request, result interface{}, closeBody bool) (*Response, error) {
	req = req.WithContext(ctx)
	resp, err := c.Sender.Send(ctx, caller, req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeBody {
			// Close the body to let the Transport reuse the connection
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	return c.ResponseParser.ParseResponse(ctx, caller, resp, result)
}

type sendOptions struct {
	// 基础 URL
	baseURL *url.URL
	// URL 中除基础 URL 外的剩余部分
	uri string
	// 请求方法
	method string

	body interface{}
	// url 查询参数
	optQuery interface{}
	// http header 参数
	optHeader interface{}
	// 用 result 反序列化 resp.Body
	result interface{}
	// 是否禁用自动调用 resp.Body.Close()
	// 自动调用 Close() 是为了能够重用连接
	disableCloseBody bool

	caller Caller
}

func (c *Client) send(ctx context.Context, opt *sendOptions) (resp *Response, err error) {
	req, err := c.newRequest(ctx, opt)
	if err != nil {
		return
	}

	resp, err = c.doAPI(ctx, opt.caller, req, opt.result, !opt.disableCloseBody)
	if err != nil {
		return
	}
	return
}

// addURLOptions adds the parameters in opt as URL query parameters to s. opt
// must be a struct whose fields may contain "url" tags.
func addURLOptions(s string, opt interface{}) (string, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	// 保留原有的参数，并且放在前面。因为 cos 的 url 路由是以第一个参数作为路由的
	// e.g. /?uploads
	q := u.RawQuery
	rq := qs.Encode()
	if q != "" {
		if rq != "" {
			u.RawQuery = fmt.Sprintf("%s&%s", q, qs.Encode())
		}
	} else {
		u.RawQuery = rq
	}
	return u.String(), nil
}

// addHeaderOptions adds the parameters in opt as Header fields to req. opt
// must be a struct whose fields may contain "header" tags.
func addHeaderOptions(header http.Header, opt interface{}) (http.Header, error) {
	v := reflect.ValueOf(opt)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return header, nil
	}

	h, err := httpheader.Header(opt)
	if err != nil {
		return nil, err
	}

	for key, values := range h {
		for _, value := range values {
			header.Add(key, value)
		}
	}
	return header, nil
}

// Owner ...
type Owner struct {
	UIN         string `xml:"uin,omitempty"`
	ID          string `xml:",omitempty"`
	DisplayName string `xml:",omitempty"`
}

// Initiator ...
type Initiator Owner

// Response API 响应
type Response struct {
	*http.Response
}

func newResponse(resp *http.Response) *Response {
	return &Response{
		Response: resp,
	}
}

var (
	xCosRequestID            = "x-cos-request-id"
	xCosTraceID              = "x-cos-trace-id"
	xCosObjectType           = "x-cos-object-type"
	xCosStorageClass         = "x-cos-storage-class"
	xCosVersionID            = "x-cos-version-id"
	xCosServerSideEncryption = "x-cos-server-side-encryption"
	xCosMetaPrefix           = "x-cos-meta-"
)

// RequestID 每次请求发送时，服务端将会自动为请求生成一个ID。
func (resp *Response) RequestID() string {
	return resp.Header.Get(xCosRequestID)
}

// TraceID 每次请求出错时，服务端将会自动为这个错误生成一个ID。
func (resp *Response) TraceID() string {
	return resp.Header.Get(xCosTraceID)
}

// ObjectType 用来表示 Object 是否可以被追加上传，枚举值：normal 或者 appendable
func (resp *Response) ObjectType() string {
	return resp.Header.Get(xCosObjectType)
}

// StorageClass Object 的存储级别，枚举值：STANDARD，STANDARD_IA
func (resp *Response) StorageClass() string {
	return resp.Header.Get(xCosStorageClass)
}

// VersionID 如果检索到的对象具有唯一的版本ID，则返回版本ID。
func (resp *Response) VersionID() string {
	return resp.Header.Get(xCosVersionID)
}

// ServerSideEncryption 如果通过 COS 管理的服务端加密来存储对象，响应将包含此头部和所使用的加密算法的值，AES256。
func (resp *Response) ServerSideEncryption() string {
	return resp.Header.Get(xCosServerSideEncryption)
}

// MetaHeaders 用户自定义的元数据
func (resp *Response) MetaHeaders() http.Header {
	h := http.Header{}
	for k := range resp.Header {
		if !strings.HasPrefix(strings.ToLower(k), xCosMetaPrefix) {
			continue
		}
		for _, v := range resp.Header[k] {
			h.Add(k, v)
		}
	}
	return h
}

// ACLHeaderOptions ...
type ACLHeaderOptions struct {
	// 定义 Object 的 acl 属性。有效值：private，public-read-write，public-read；默认值：private
	XCosACL string `header:"x-cos-acl,omitempty" url:"-" xml:"-"`
	// 	赋予被授权者读的权限。格式：id="[OwnerUin]"
	XCosGrantRead string `header:"x-cos-grant-read,omitempty" url:"-" xml:"-"`
	// 赋予被授权者写的权限。格式：id="[OwnerUin]"
	XCosGrantWrite string `header:"x-cos-grant-write,omitempty" url:"-" xml:"-"`
	// 赋予被授权者所有的权限。格式：id="[OwnerUin]"
	XCosGrantFullControl string `header:"x-cos-grant-full-control,omitempty" url:"-" xml:"-"`
}

// ACLGrantee ...
type ACLGrantee struct {
	Type        string `xml:"type,attr"`
	UIN         string `xml:"uin,omitempty"`
	ID          string `xml:",omitempty"`
	DisplayName string `xml:",omitempty"`
	SubAccount  string `xml:"Subaccount,omitempty"`
}

// ACLGrant ...
type ACLGrant struct {
	Grantee *ACLGrantee
	// 指明授予被授权者的权限信息，枚举值：READ，WRITE，FULL_CONTROL
	Permission string
}

// ACLXml ...
//
// https://cloud.tencent.com/document/product/436/7733
type ACLXml struct {
	XMLName           xml.Name `xml:"AccessControlPolicy"`
	Owner             *Owner
	AccessControlList []ACLGrant `xml:"AccessControlList>Grant,omitempty"`
}

const (
	// StorageClassStandard Object 的存储级别: STANDARD
	StorageClassStandard string = "STANDARD"
	// StorageClassStandardTA Object 的存储级别: STANDARD_IA
	StorageClassStandardTA string = "STANDARD_IA"
	// StorageClassArchive Object 的存储级别: ARCHIVE
	StorageClassArchive string = "ARCHIVE"

	// ObjectTypeAppendable : appendable
	ObjectTypeAppendable string = "appendable"
	// ObjectTypeNormal : normal
	ObjectTypeNormal string = "normal"

	// ServerSideEncryptionAES256 服务端加密算法: AES256
	ServerSideEncryptionAES256 string = "AES256"

	// PermissionRead 权限值: READ
	PermissionRead string = "READ"
	// PermissionWrite 权限值: WRITE
	PermissionWrite string = "WRITE"
	// PermissionFullControl  权限值: FULL_CONTROL
	PermissionFullControl string = "FULL_CONTROL"
)
