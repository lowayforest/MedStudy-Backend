package folder

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeFolderRepository 是满足 Repository 接口的假仓储，只记录调用并返回预设结果。
// 它不执行 SQL、不读取配置、不访问网络；用于单独检查 Service 的业务编排。
type fakeFolderRepository struct {
	calls   int
	ctx     context.Context
	userID  string
	input   CreateInput
	created Folder
	err     error
}

func (r *fakeFolderRepository) Create(ctx context.Context, userID string, input CreateInput) (Folder, error) {
	r.calls++
	r.ctx = ctx
	r.userID = userID
	r.input = input
	return r.created, r.err
}

// TestServiceCreateSuccess 验证名称去空格后才交给仓储，ctx 和用户 ID 原样传递。
// 假仓储返回固定 ID 和时间，Service 应完整返回它们，而不是自己重新生成。
func TestServiceCreateSuccess(t *testing.T) {
	tests := []struct {
		name string
		kind string
	}{
		// 资料类型：允许 material，中文名称及内部空格应保留。
		{name: "资料类型", kind: "material"},
		// 真题类型：允许 exam，不能把所有创建都固定成 material。
		{name: "真题类型", kind: "exam"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const userID = "00000000-0000-4000-8000-000000000002"
			input := CreateInput{Name: "\u3000\t口腔 解剖 \n", Type: test.kind}
			want := Folder{
				ID: "00000000-0000-4000-8000-000000000010", UserID: userID,
				Name: "口腔 解剖", Type: test.kind,
				CreatedAt: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
			}
			repo := &fakeFolderRepository{created: want}

			created, err := NewService(repo).Create(ctx, userID, input)
			if err != nil {
				t.Fatalf("合法输入创建失败：%v", err)
			}
			if repo.calls != 1 || repo.ctx != ctx || repo.userID != userID {
				t.Fatal("应该只调用一次仓储，并原样传递请求 context 和用户 ID")
			}
			if repo.input != (CreateInput{Name: "口腔 解剖", Type: test.kind}) {
				t.Fatal("仓储应接收去掉首尾空白、保留内部空格的名称和原类型")
			}
			if created != want {
				t.Fatal("Service 应完整返回仓储提供的资料夹，包括 ID 和创建时间")
			}
		})
	}
}

// TestServiceCreateRejectsInvalidInput 验证非法输入在调用仓储之前被拒绝。
// 只检查业务编排，不重复全部长度边界；详细边界由 validation_test.go 覆盖。
func TestServiceCreateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input CreateInput
		err   error
	}{
		// 去空格后为空：名称不合法，应该返回名称错误且不调用仓储。
		{name: "空白名称", input: CreateInput{Name: " \t\n", Type: "material"}, err: ErrInvalidName},
		// 名称合法但类型非法：应该返回类型错误且不调用仓储。
		{name: "非法类型", input: CreateInput{Name: "资料", Type: "other"}, err: ErrInvalidType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeFolderRepository{}
			created, err := NewService(repo).Create(context.Background(), "fixture-user", test.input)
			if !errors.Is(err, test.err) {
				t.Fatalf("期望校验错误 %v，实际 %v", test.err, err)
			}
			if repo.calls != 0 {
				t.Fatal("校验失败不能调用仓储写入数据")
			}
			if created != (Folder{}) {
				t.Fatal("校验失败应返回零值 Folder")
			}
		})
	}
}

// TestServiceCreateRepositoryFailure 验证仓储错误不被吞掉，也不返回其部分结果。
// 假仓储故意同时返回对象和错误；Service 必须把这次调用视为失败。
func TestServiceCreateRepositoryFailure(t *testing.T) {
	wantErr := errors.New("fixture-only-storage-failure")
	repo := &fakeFolderRepository{created: Folder{ID: "partial-fixture-id"}, err: wantErr}
	created, err := NewService(repo).Create(context.Background(), "fixture-user", CreateInput{
		Name: "口腔解剖", Type: "material",
	})
	if repo.calls != 1 || !errors.Is(err, wantErr) {
		t.Fatal("合法输入应该调用仓储一次，并保留仓储返回的错误原因")
	}
	if created != (Folder{}) {
		t.Fatal("仓储失败不能把部分对象作为成功结果返回")
	}
}
