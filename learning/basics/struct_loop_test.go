package basics

import "testing"

// TestPractice 是结构体、切片和循环的学习练习。
// 两种循环应输出相同的学生列表；这里只通过日志观察，没有自动断言，不算业务验收。
// 运行：go test -v ./learning/basics -run '^TestPractice$'
func TestPractice(t *testing.T) {

	// TODO 1：在这个函数中定义 Student 结构体。
	// 字段：Name，类型 string；Age，类型 int。
	type Student struct {
		Name string
		Age  int
	}

	// TODO 2：创建一个 Student 值：姓名“张三”，年龄 18。
	// 用 t.Log 输出这个值的 Name 和 Age 字段。
	student1 := Student{Name: "张三", Age: 18}
	t.Log(student1.Name, student1.Age)

	// TODO 3：创建名为 students 的 []Student 切片，包含两个学生：
	// 张三，18 岁；李四，20 岁。
	students := []Student{
		{
			Name: "张三",
			Age:  18,
		},
		{
			Name: "李四",
			Age:  20,
		},
	}

	// TODO 4：使用 for ... range 遍历 students。
	// 每轮用 t.Log 输出当前学生的索引、姓名和年龄。

	for index, student := range students {
		t.Log(index, student.Name, student.Age)
	}

	// TODO 5：使用三段式 for 循环，再遍历一次 students。
	// 每轮用 t.Log 输出当前学生的索引、姓名和年龄。
	// 提示：len(students) 得到元素数量，students[i] 取得第 i 个元素。
	for i := 0; i < len(students); i++ {
		t.Log(i, students[i].Name, students[i].Age)
	}

	// 两种循环都应依次输出：
	// 0 张三 18
	// 1 李四 20
}
