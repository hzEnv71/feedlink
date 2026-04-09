package services

import "testing"

func TestFeedService_mergeAndDedup(t *testing.T) {
	svc := &FeedService{}

	result := svc.mergeAndDedup(
		[]uint{1, 2, 3},
		[]uint{3, 4, 5},
		[]uint{2, 6},
	)

	want := []uint{1, 2, 3, 4, 5, 6}
	if len(result) != len(want) {
		t.Fatalf("unexpected length: got=%d want=%d", len(result), len(want))
	}

	for i := range want {
		if result[i] != want[i] {
			t.Fatalf("unexpected value at %d: got=%d want=%d", i, result[i], want[i])
		}
	}
}

func TestFeedService_GetTimeline_PaginationSkeleton(t *testing.T) {
	// 说明：
	// 该测试为骨架，用于后续补充时间线分页行为的完整验证。
	// 建议后续引入 mock repository/cache 后覆盖：
	// 1) 空列表分页
	// 2) page 越界
	// 3) 正常分页（第1页/第N页）
	// 4) 合并去重后再分页的稳定性
	//
	// 目前先保留骨架，防止误报并作为扩展锚点。
	t.Skip("TODO: add timeline pagination tests with mocked repositories/cache")
}
