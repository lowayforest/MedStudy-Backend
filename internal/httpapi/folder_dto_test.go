package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lowayforest/MedStudy-Backend/internal/folder"
)

// TestNewFolderResponse 验证业务对象转换成约定的 HTTP JSON，而非仅检查字段声明。
// 时间带毫秒和非 UTC 时区：必须输出原时间点的毫秒整数；UserID 不能出现在响应中。
// 这里只执行转换和 JSON 编码，不发送 HTTP 请求、不连接数据库。
func TestNewFolderResponse(t *testing.T) {
	value := folder.Folder{
		ID:        "00000000-0000-4000-8000-000000000010",
		UserID:    "00000000-0000-4000-8000-000000000002",
		Name:      "口腔解剖",
		Type:      "material",
		CreatedAt: time.Unix(1787712000, 123000000).In(time.FixedZone("fixture-zone", 8*60*60)),
	}
	const wantMillis int64 = 1787712000123

	// 检查转换保留公开字段，时间单位是毫秒而不是秒或纳秒。
	response := newFolderResponse(value)
	want := folderResponse{ID: value.ID, Name: value.Name, Type: value.Type, CreatedAt: wantMillis}
	if response != want {
		t.Fatal("响应应保留 ID、名称、类型，并正确转换毫秒时间戳")
	}

	// 实际编码后检查字段名，才能发现 Go 编译无法识别的 JSON 标签错误。
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("编码资料夹响应失败：%v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("解析测试响应失败：%v", err)
	}
	if len(fields) != 4 {
		t.Fatal("资料夹响应只能包含四个公开字段，不能泄露 UserID 或加入额外字段")
	}
	for _, key := range []string{"id", "name", "type", "createdAt"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("响应缺少契约字段 %q，请检查 JSON 标签：%s", key, data)
		}
	}

	// 再检查 JSON 中确实是整数，不能输出带引号的数字或日期字符串。
	var millis int64
	if err := json.Unmarshal(fields["createdAt"], &millis); err != nil || millis != wantMillis {
		t.Fatal("createdAt 必须是正确的毫秒整数，不能是秒数或字符串")
	}
}
