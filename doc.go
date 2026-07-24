// Package apsara 是阿里云专有云 (Apsara V3.18.6) 通用 Go SDK。
//
// 该 SDK 不定义各接口的 struct，仅提供认证、请求构建和响应序列化的通用基础设施。
// 一个 Client 实例可复用于所有产品，每次调用时传入 action（操作名）和 version（API 版本号）。
//
// # 快速开始
//
//	import "github.com/gomodb/apsara"
//
//	client, err := apsara.NewClient(
//	    apsara.WithEndpoint("ecs.aliyuncs.com"),
//	    apsara.WithRegion("cn-hangzhou"),
//	    apsara.WithCredential(apsara.Credential{
//	        AccessKeyID:     os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"),
//	        AccessKeySecret: os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"),
//	    }),
//	    apsara.WithTimeout(30 * time.Second),
//	)
//	if err != nil { panic(err) }
//
//	var resp map[string]any
//	err = client.Get(ctx, "DescribeInstances", "2014-05-26",
//	    map[string]string{"PageSize": "10"}, &resp)
//
// # 从环境变量加载凭证
//
//	export APSARA_ACCESS_KEY_ID=xxx
//	export APSARA_ACCESS_KEY_SECRET=yyy
//	export APSARA_SECURITY_TOKEN=zzz  # STS 可选
//
//	client, err := apsara.NewClient(
//	    apsara.WithEndpoint("ecs.aliyuncs.com"),
//	    apsara.WithRegion("cn-hangzhou"),
//	    apsara.WithTimeout(30 * time.Second),
//	)
//
// # 专有云完整配置（超时 + 自签名证书 + 重试 + 组织/资源集）
//
//	client, err := apsara.NewClient(
//	    apsara.WithEndpoint("ecs.aliyuncs.com"),
//	    apsara.WithRegion("cn-hangzhou"),
//	    apsara.WithTimeout(30 * time.Second),
//	    apsara.WithInsecureSkipVerify(true),
//	    apsara.WithOrganizationID("org-xxx"),
//	    apsara.WithResourceGroupID("rg-xxx"),
//	    apsara.WithRetry(3),
//	)
//
// # 错误处理
//
//	var ae *apsara.ApsaraError
//	if errors.As(err, &ae) {
//	    fmt.Printf("status=%d request=%s code=%s msg=%s\n",
//	        ae.StatusCode, ae.RequestID, ae.Code, ae.Message)
//	}
//
// # 获取响应元数据
//
//	var meta apsara.ResponseMeta
//	err = client.Get(ctx, "DescribeInstances", "2014-05-26", nil, &resp,
//	    apsara.WithMeta(&meta))
//	fmt.Println("RequestId:", meta.RequestID)
package apsara
