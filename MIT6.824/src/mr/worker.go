package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"sort"
	"time"
)

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// for sorting by key.
type ByKey []KeyValue

func (a ByKey) Len() int			{ return len(a) }
func (a ByKey) Swap(i, j int )		{ a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool 	{ return a[i].Key < a[j].Key }

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue, reducef func(string, []string) string) {

	// Worker get started.
	for {
		task := getTaskFromMaster()

		switch task.TaskState {
		case Map:
			mapper(&task, mapf)
		case Reduce:
			reducer(&task, reducef)
		case Wait:
			time.Sleep(5 * time.Second)
		case Exit:
			return
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
		os.Exit(0)
		// log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

func getTaskFromMaster() Task {
	args := TaskReq{}
	reply := Task{}
	call("Master.AssignTask", &args, &reply)
	return reply
}

func mapper(task *Task, mapf func(string, string) []KeyValue) {
	// Read from the file of the task.
	content, err := ioutil.ReadFile(task.Input)
	if err != nil {
		log.Fatal("Failed to read file: " + task.Input, err)
	}
	// Call the map function defined by user.
	intermediates := mapf(task.Input, string(content))
	
	buffer := make([][]KeyValue, task.NReducer)
	for _, intermediate := range intermediates {
		slot := ihash(intermediate.Key) % task.NReducer
		buffer[slot] = append(buffer[slot], intermediate)
	}
	
	// Cache the result in local disk.
	// The list of local files. 
	mapOutput := make([]string, 0) // [mr-MapId-1, mr-MapId-2, ... , mr-MapId-NReduce]
	for i := 0; i < task.NReducer; i++ {
		mapOutput = append(mapOutput, writeToLocalFile(task.Id, i, &buffer[i]))
	}

	task.InterMediates = mapOutput
	taskCompleted(task)
}

func reducer(task *Task, reducef func(string, []string) string) {
	intermediate := *readFromLocalFile(task.InterMediates)
	sort.Sort(ByKey(intermediate))
	dir, _ := os.Getwd()
	tempFile, err := ioutil.TempFile(dir, "mr-tmp-*")
	if err != nil {
		log.Fatal("Failed to create temp file: ", err)
	}
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[j-1].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)
		fmt.Fprintf(tempFile, "%v %v\n", intermediate[i].Key, output)
		i = j
	}
	tempFile.Close()
	oname := fmt.Sprintf("mr-out-%d", task.Id)
	os.Rename(tempFile.Name(), oname)
	task.Output = oname
	taskCompleted(task)
}

// writeToLocalFile write the intermediates to local disk.
// Return the absolute path of the local file.
func writeToLocalFile(MapId, reduceId int, kvs *[]KeyValue) string {
	dir, _ := os.Getwd()
	tempFile, err := ioutil.TempFile(dir, "mr-tmp-*")
	if err != nil {
		log.Fatal("Failed to create temp file", err)
	}
	enc := json.NewEncoder(tempFile)
	for _, kv := range *kvs {
		if err := enc.Encode(&kv); err != nil {
			log.Fatal("Failed to write kv pair", err)
		}
	}
	tempFile.Close()
	outputName := fmt.Sprintf("mr-%d-%d", MapId, reduceId)	
	os.Rename(tempFile.Name(), outputName)
	return filepath.Join(dir, outputName)
}

func readFromLocalFile(files []string) *[]KeyValue {
	// log.Println("read from local file: ", files)
	kva := []KeyValue{}
	for _, filepath := range files {
		file, err := os.Open(filepath)
		if err != nil {
			log.Fatal("Failed to open file " + filepath, err)
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			kva = append(kva, kv)
		}
	}
	return &kva
}

// taskCompleted: Call the master when a task finish.
func taskCompleted(task *Task) {
	reply := ExampleReply{}	// the reply is not importance.
	call("Master.TaskCompleted", task, &reply)
}

