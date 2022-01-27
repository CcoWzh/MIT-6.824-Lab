package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	// Your worker implementation here.
	// work需要不断的向其请求,要任务
	for {
		task := CallTask()
		switch task.State {
		case _map:
			workerMap(mapf, task)
			break
		case _reduce:
			workerReduce(reducef, task)
			break
		case _wait:
			// wait for 5 seconds to requeset again
			time.Sleep(time.Duration(time.Second * 5))
			break
		case _end:
			fmt.Println("End...")
			return
		default:
			log.Fatalln("无效状态!!!")
		}
	}

	// uncomment to send the Example RPC to the master.
	// CallExample()

}

//
// example function to show how to make an RPC call to the master.
//
// the RPC argument and reply types are defined in rpc.go.
//
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	call("Master.Example", &args, &reply)

	// reply.Y should be 100.
	fmt.Printf("reply.Y %v\n", reply.Y)
}

// 向master要任务
func CallTask() *TaskState {
	args := ExampleArgs{}
	reply := TaskState{}
	call("Master.HandleTaskRequest", &args, &reply)
	return &reply
}

// 如果是 map 任务完成，则 master 将 map 任务移除
// 如果是 reduce 任务完成，则 master 将 reduce 任务移除
func CallTaskDone(taskInfo *TaskState) {
	reply := ExampleReply{}
	call("Master.TaskDone", taskInfo, &reply)
}

//
// send an RPC request to the master, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := masterSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

func workerMap(mapf func(string, string) []KeyValue, taskInfo *TaskState) {
	fmt.Printf("Got assigned map task on %vth file %s\n", taskInfo.MapIndex, taskInfo.FileName)

	// read in target files as a key-value array
	intermediate := []KeyValue{}
	file, err := os.Open(taskInfo.FileName)
	if err != nil {
		log.Fatalf("cannot open %v", taskInfo.FileName)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", taskInfo.FileName)
	}
	file.Close()
	kva := mapf(taskInfo.FileName, string(content))
	intermediate = append(intermediate, kva...)

	// prepare output files and encoders
	nReduce := taskInfo.R
	outprefix := "mr-tmp/mr-"
	outprefix += strconv.Itoa(taskInfo.MapIndex)
	outprefix += "-"
	outFiles := make([]*os.File, nReduce)
	fileEncs := make([]*json.Encoder, nReduce)
	for outindex := 0; outindex < nReduce; outindex++ {
		//outname := outprefix + strconv.Itoa(outindex)
		//outFiles[outindex], _ = os.Create(outname)
		outFiles[outindex], _ = ioutil.TempFile("mr-tmp", "mr-tmp-*")
		fileEncs[outindex] = json.NewEncoder(outFiles[outindex])
	}

	// distribute keys among mr-fileindex-*
	for _, kv := range intermediate {
		outindex := ihash(kv.Key) % nReduce
		file = outFiles[outindex]
		enc := fileEncs[outindex]
		err := enc.Encode(&kv)
		if err != nil {
			fmt.Printf("File %v Key %v Value %v Error: %v\n", taskInfo.FileName, kv.Key, kv.Value, err)
			panic("Json encode failed")
		}
	}

	// save as files
	for outindex, file := range outFiles {
		outname := outprefix + strconv.Itoa(outindex)
		oldpath := filepath.Join(file.Name())
		//fmt.Printf("temp file oldpath %v\n", oldpath)
		os.Rename(oldpath, outname)
		file.Close()
	}
	// acknowledge master
	// todo taskInfo.State how to update ?
	CallTaskDone(taskInfo)
}

func workerReduce(reducef func(string, []string) string, taskInfo *TaskState) {
	fmt.Printf("Got assigned reduce task on part %v\n", taskInfo.ReduceIndex)
	outname := "mr-out-" + strconv.Itoa(taskInfo.ReduceIndex)
	//fmt.Printf("%v\n", taskInfo)

	// read from output files from map tasks

	innameprefix := "mr-tmp/mr-"
	innamesuffix := "-" + strconv.Itoa(taskInfo.ReduceIndex)

	// read in all files as a kv array
	intermediate := []KeyValue{}
	for index := 0; index < taskInfo.M; index++ {
		inname := innameprefix + strconv.Itoa(index) + innamesuffix
		file, err := os.Open(inname)
		if err != nil {
			fmt.Printf("Open intermediate file %v failed: %v\n", inname, err)
			panic("Open file error")
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			intermediate = append(intermediate, kv)
		}
		file.Close()
	}

	sort.Sort(ByKey(intermediate))

	//ofile, err := os.Create(outname)
	ofile, err := ioutil.TempFile("mr-tmp", "mr-*")
	if err != nil {
		fmt.Printf("Create output file %v failed: %v\n", outname, err)
		panic("Create file error")
	}
	//fmt.Printf("%v\n", intermediate)
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)

		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)

		i = j
	}
	os.Rename(filepath.Join(ofile.Name()), outname)
	ofile.Close()
	// acknowledge master
	CallTaskDone(taskInfo)
}
