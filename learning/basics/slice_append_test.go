package basics

import "testing"

// TestPracticeAppend 练习使用 append 向切片追加元素。
// 运行：go test -v ./learning/basics -run '^TestPracticeAppend$'
func TestPracticeAppend(t *testing.T) {

	type Student struct {
		Name string
		Age  int
	}

	students := []Student{
		{Name: "张三", Age: 18},
		{Name: "李四", Age: 20},
	}
	newStudent := Student{Name: "王五", Age: 22}

	t.Logf("追加前长度：%v", len(students))

	// TODO：使用 append 把 newStudent 添加到 students 的末尾。
	// 注意：要把 append 返回的切片赋回 students。
	students = append(students, newStudent)

	t.Logf("追加后长度：%v", len(students))

	// 以下检查已准备好，你只需要填写上面的 TODO。
	if len(students) != 3 {
		t.Fatalf("期望追加后有 3 个学生，实际有 %v 个", len(students))
	}
	if students[2].Name != newStudent.Name || students[2].Age != newStudent.Age {
		t.Fatalf("第三个学生不符合预期：%v", students[2])
	}
	for index, student := range students {
		t.Log(index, student.Name, student.Age)
	}
}
