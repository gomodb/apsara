package apsara_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gomodb/apsara"
)

func Example() {
	ctx := context.Background()

	// 创建客户端（超时 + 自签名证书 + 重试）
	client, err := apsara.NewClient(
		apsara.WithEndpoint("ecs.aliyuncs.com"),
		apsara.WithRegion("cn-hangzhou"),
		apsara.WithCredential(apsara.Credential{
			AccessKeyID:     "your-access-key-id",
			AccessKeySecret: "your-access-key-secret",
		}),
		apsara.WithTimeout(30*time.Second),  // 请求超时 30s
		apsara.WithInsecureSkipVerify(true), // 自签名证书
		apsara.WithOrganizationID("org-xxx"),
		apsara.WithResourceGroupID("rg-xxx"),
		apsara.WithRetry(3), // 最多重试 3 次
	)
	if err != nil {
		fmt.Printf("create client: %v\n", err)
		return
	}

	// 调用 ECS API，附带获取元数据
	var (
		resp map[string]any
		meta apsara.ResponseMeta
	)

	err = client.Get(ctx,
		"DescribeInstances", "2014-05-26",
		map[string]string{"PageSize": "5"},
		&resp,
		apsara.WithMeta(&meta),
	)
	if err != nil {
		var ae *apsara.ApsaraError
		if errors.As(err, &ae) {
			fmt.Printf("API error: status=%d request=%s code=%s\n",
				ae.StatusCode, ae.RequestID, ae.Code)
		} else {
			fmt.Printf("Error: %v\n", err)
		}

		return
	}

	fmt.Printf("RequestId: %s\n", meta.RequestID)
	fmt.Printf("Total: %v\n", resp["TotalCount"])

	// 同一 client 调用 VPC（需不同 endpoint 时新建 client）
	vpcClient, _ := apsara.NewClient(
		apsara.WithEndpoint("vpc.aliyuncs.com"),
		apsara.WithRegion("cn-hangzhou"),
		apsara.WithCredential(apsara.Credential{
			AccessKeyID:     "your-access-key-id",
			AccessKeySecret: "your-access-key-secret",
		}),
	)

	var vpcResp map[string]any

	err = vpcClient.Get(ctx, "DescribeVpcs", "2016-04-28", nil, &vpcResp)
	if err != nil {
		fmt.Printf("VPC Error: %v\n", err)
		return
	}

	fmt.Printf("Vpcs: %v\n", vpcResp["Vpcs"])
}
