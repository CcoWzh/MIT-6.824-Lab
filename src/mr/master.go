package mr

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

type Task struct {
	// 需要考虑到任务超时的情况
	beginTime time.Time
	// 任务编号
	mapIndex int
	// 是reduce worker的编号，为了之后方便存储
	reduceIndex int
	// 文件名，用于map程序读取文件
	FileName string
	// 是map之后分成多少份的reudce，即reduce 程序的个数
	R int
	// 是map 程序的总数，即，文件的总数
	M int
}

// 计算时差
func (t *Task) TimeOut() bool {
	return time.Now().Sub(t.beginTime) > time.Second*10
}

// 设置时差
func (t *Task) SetTime() {
	t.beginTime = time.Now()
}

type TaskQueue struct {
	taskArray []Task
	// 需要使用锁，避免冲突
	mutex sync.Mutex
}

func (t *TaskQueue) Size() int {
	return len(t.taskArray)
}

// 出队列
func (t *TaskQueue) Pop() (Task, error) {
	t.mutex.Lock()
	taskSize := len(t.taskArray)
	if taskSize == 0 {
		t.mutex.Unlock()
		return Task{}, errors.New("queue is empty")
	}
	popTask := t.taskArray[taskSize-1]
	t.taskArray = t.taskArray[:taskSize-1]
	t.mutex.Unlock()
	return popTask, nil
}

// 入队列
func (t *TaskQueue) Push(task Task) {
	t.mutex.Lock()
	// task == nil ?
	if reflect.DeepEqual(task, Task{}) {
		t.mutex.Unlock()
		return
	}
	t.taskArray = append(t.taskArray, task)
	t.mutex.Unlock()
}

func (t *Task) GenerateTaskInformation(s int) TaskState {
	state := TaskState{}
	state.M = t.M
	state.MapIndex = t.mapIndex
	state.ReduceIndex = t.reduceIndex
	state.R = t.R
	state.FileName = t.FileName
	switch s {
	case _map:
		state.State = _map
	case _reduce:
		state.State = _reduce
	}

	return state
}

// 获得超时的任务列表
func (t *TaskQueue) TimeOut() []Task {
	outArray := make([]Task, 0)
	t.mutex.Lock()
	for index := 0; index < len(t.taskArray); {
		task := t.taskArray[index]
		if task.TimeOut() {
			outArray = append(outArray, task)
			// 删除
			t.taskArray = append(t.taskArray[:index], t.taskArray[index+1:]...)
		} else {
			index++
		}
	}
	t.mutex.Unlock()
	return outArray
}

// 如果这个任务完成了，需要将该任务从队列中移除
func (t *TaskQueue) RemoveTask(fileIndex, partIndex int) {
	t.mutex.Lock()
	// 由于是并发的，不知道哪个任务先完成，需要根据任务序号移除
	for index := 0; index < len(t.taskArray); {
		task := t.taskArray[index]
		if fileIndex == task.mapIndex && partIndex == task.reduceIndex {
			t.taskArray = append(t.taskArray[:index], t.taskArray[index+1:]...)
			break
		} else {
			index++
		}
	}
	t.mutex.Unlock()
}

//type MapTaskStat struct {
//	Task
//}
//
//type ReduceTaskStat struct {
//	Task
//}

type Master struct {
	// Your definitions here.
	// file list
	fileName []string

	// map and reduce task state
	// 维护任务队列，指明哪些任务未分配、哪些已分配正在运行、哪些已完成
	// idle:1,in-progress:2,completed:3
	// 这3个状态，应该是3个队列，放在Master中管理
	idleMapTask       TaskQueue // 等待执行的任务队列
	inProgressMapTask TaskQueue // 已分配、未完成队列,即,正在执行的任务队列
	// if len(inProgressMapTask) == 0 {map task is over }
	idleReduceTask       TaskQueue
	inProgressReduceTask TaskQueue

	// other state
	isDone bool
	R      int
}

// Your code here -- RPC handlers for the worker to call.

// 处理来自worker的task请求
// 这是最关键的函数!!!
func (m *Master) HandleTaskRequest(args *ExampleArgs, reply *TaskState) error {
	if m.isDone {
		return nil
	}
	// 是否有未完成的map或reduce任务
	reduceTask, err := m.idleReduceTask.Pop()
	if err == nil {
		reduceTask.SetTime()
		// 压入任务执行的队列
		m.inProgressMapTask.Push(reduceTask)
		// 和 worker map 程序通信
		*reply = reduceTask.GenerateTaskInformation(_reduce)
		fmt.Println("分配了一个任务给reduce执行...")
		return nil
	}

	mapTask, err := m.idleMapTask.Pop()
	if err == nil {
		mapTask.SetTime()
		// 压入任务执行的队列
		m.inProgressMapTask.Push(mapTask)
		// 和 worker map 程序通信
		*reply = mapTask.GenerateTaskInformation(_map)
		fmt.Println("分配了一个任务给map执行...")
		return nil
	}
	// 若有未分配的map任务,即map任务队列长度非0,则给worker分配这个任务,并标记这个任务为已分配、未完成
	if m.inProgressMapTask.Size() > 0 || m.inProgressReduceTask.Size() > 0 {
		// must wait for new tasks
		reply.State = _wait
		return nil
	}
	// 所有的任务都结束了
	reply.State = _end
	m.isDone = true
	return nil
}

// 处理来自worker的task请求
// 如果有mao或者reduce任务完成了，就处理掉
func (m *Master) TaskDone(args *TaskState, reply *ExampleReply) error {
	switch args.State {
	case 0:
		// 如果是 map 任务完成，则 map 将 reduce 任务移除
		fmt.Printf("Map task on %vth file %v complete\n", args.MapIndex, args.FileName)
		m.inProgressMapTask.RemoveTask(args.MapIndex, args.ReduceIndex)
		if m.inProgressMapTask.Size() == 0 && m.idleMapTask.Size() == 0 {
			// 当所有的 map 任务都完成了，开始分配 reduce 任务
			m.distributeReduceTask()
		}
		break
	case 1:
		// 如果是 reduce 任务完成，则 master 将 reduce 任务移除
		fmt.Printf("Reduce task on %vth part complete\n", args.ReduceIndex)
		m.inProgressReduceTask.RemoveTask(args.MapIndex, args.ReduceIndex)
		break
	default:
		panic("Task Done error")
	}
	return nil
}

// 分配 reduce 任务
func (m *Master) distributeReduceTask() {
	// 根据 master 的 R，定义R个reduce个任务
	for i := 0; i < m.R; i++ {
		task := Task{
			mapIndex:    0,
			reduceIndex: i,
			FileName:    "",
			R:           m.R,
			M:           len(m.fileName),
		}

		m.idleReduceTask.Push(task)
	}
}

//
// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
//
func (m *Master) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrmaster.go calls Done() periodically to find out
// if the entire job has finished.
//
func (m *Master) Done() bool {
	return m.isDone
}

// 每隔5秒，检查是否有超时的任务
func (m *Master) findTaskTimeOut() {
	for {
		time.Sleep(5 * time.Second)
		mapTask := m.inProgressMapTask.TimeOut()
		if len(mapTask) > 0 {
			for _, v := range mapTask {
				m.idleMapTask.Push(v)
			}
			mapTask = nil
		}
		mapReduce := m.inProgressReduceTask.TimeOut()
		if len(mapReduce) > 0 {
			for _, v := range mapReduce {
				m.idleReduceTask.Push(v)
			}
			mapReduce = nil
		}
	}
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	//m := Master{}

	// Your code here.
	// 将 map 任务分割好
	// 相当于，一个文件名一个任务??
	mapArray := make([]Task, 0)
	for fileIndex, fileName := range files {
		mapTask := Task{
			mapIndex:    fileIndex,
			reduceIndex: 0,
			FileName:    fileName,
			R:           nReduce,
			M:           len(files),
		}
		mapArray = append(mapArray, mapTask)
	}

	// create tmp directory if not exists
	if _, err := os.Stat("mr-tmp"); os.IsNotExist(err) {
		err = os.Mkdir("mr-tmp", os.ModePerm)
		if err != nil {
			fmt.Print("Create tmp directory failed... Error: %v\n", err)
			panic("Create tmp directory failed...")
		}
	}

	m := Master{
		fileName:    files,
		idleMapTask: TaskQueue{taskArray: mapArray},
		isDone:      false,
		R:           nReduce,
	}
	// 需要考虑到任务超时的情况
	go m.findTaskTimeOut()
	// 开启端口监听，用于进程间通信
	m.server()
	return &m
}
