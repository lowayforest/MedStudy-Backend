package basics

import "testing"

// TestPracticeRangeUpdate 练习通过循环修改切片中的结构体。
// 从年龄 18、20 开始，循环加一后断言原切片中的年龄变为 19、21。
// 学习重点：range 取出的结构体值是副本；通过索引修改切片中的元素。
// 运行：go test -v ./learning/basics -run '^TestPracticeRangeUpdate$'
func TestPracticeRangeUpdate(t *testing.T) {
	type Student struct {
		Name string
		Age  int
	}

	students := []Student{
		{Name: "张三", Age: 18},
		{Name: "李四", Age: 20},
	}

	t.Logf("修改前：%v", students)

	// 通过索引给切片中每个学生的年龄加一。
	for index := range students {
		students[index].Age += 1
	}

	t.Logf("修改后：%v", students)

	// 不要修改期望结果；年龄应从 18、20 变成 19、21。
	if students[0].Age != 19 || students[1].Age != 21 {
		t.Fatalf("期望年龄分别为 19、21，实际得到 %v、%v", students[0].Age, students[1].Age)
	}
}
