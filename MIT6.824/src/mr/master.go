package mr

import (
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type MasterTaskState int

const (
	Idle MasterTaskState = iota
	InProgress
	Completed
)

type State int

const (
	Map State = iota
	Reduce
	Exit
	Wait
)

var mu sync.Mutex

type Master struct {
	TaskQueue 		chan *Task			// Task todo channel.
	TaskMeta 		map[int]*MasterTask
	Phase  			State	
	NReduce			int
	InputFiles 		[]string
	// InterMediates cache all the intermediate result.
	// [1] ['mr-1-1', 'mr-2-1', 'mr-3-1']
	// [2] ['mr-1-2', 'mr-2-2', 'mr-3-2']
	InterMediates 	[][]string	
}

type Task struct {
	Id				int
	Input 			string
	Output			string
	TaskState 		State
	NReducer 		int
	InterMediates 	[]string	
}

type MasterTask struct {
	TaskStatus	MasterTaskState
	StartTime	time.Time
	TaskPtr		*Task
}

// Your code here -- RPC handlers for the worker to call.

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
	mu.Lock()
	defer mu.Unlock()
	ret := m.Phase == Exit
	return ret
}

//
// create a Master.
// main/mrmaster.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeMaster(files []string, nReduce int) *Master {
	m := Master{
		TaskQueue: 		make(chan *Task, max(nReduce, len(files))),
		TaskMeta: 		make(map[int]*MasterTask),
		Phase: 			Map,
		NReduce: 		nReduce,
		InputFiles: 	files,
		InterMediates: 	make([][]string, nReduce),
	}

	// Create a Map task for each input files when the master start.
	m.createMapTask()
	
	// Run master server.
	m.server()
	go m.catchTimeOut()
	return &m
}

func (m *Master) catchTimeOut() {
	for {
		time.Sleep(5 * time.Second)
		mu.Lock()
		if m.Phase == Exit {
			mu.Unlock()
			return
		}
		for _, t := range m.TaskMeta {
			if t.TaskStatus == InProgress && time.Now().Sub(t.StartTime) > 10 * time.Second {
				m.TaskQueue <- t.TaskPtr
				t.TaskStatus = Idle
			}
		}
		mu.Unlock()
	}
}

func (m *Master) createMapTask() {
	for idx, filename := range m.InputFiles {
		task := Task{
			Id: idx,
			Input: filename,
			TaskState: Map,
			NReducer: m.NReduce,
		}
		m.TaskQueue <- &task
		m.TaskMeta[idx] = &MasterTask{
			TaskStatus: Idle,
			TaskPtr: 	&task,
		}
	}
}

func (m *Master) createReduceTask() {
	m.TaskMeta = make(map[int]*MasterTask) // refresh the TaskMeta map prepare for Reduce task.
	for idx, files := range m.InterMediates {
		task := Task{
			Id: idx,
			TaskState: Reduce,
			NReducer: m.NReduce,
			InterMediates: files,
		}
		m.TaskQueue <- &task
		m.TaskMeta[idx] = &MasterTask{
			TaskStatus: Idle,
			TaskPtr: &task,
		}
	}
}

// AssignTask: Assign task to the worker if has task in the TaskQueue.
func (m *Master) AssignTask(args *TaskReq, reply *Task) error {
	mu.Lock()
	defer mu.Unlock()
	if len(m.TaskQueue) > 0 {
		*reply = *<-m.TaskQueue
		m.TaskMeta[reply.Id].TaskStatus = InProgress
		m.TaskMeta[reply.Id].StartTime = time.Now()
	} else if m.Phase == Exit {
		*reply = Task{TaskState: Exit}
	} else {
		*reply = Task{TaskState: Wait}
	}
	return nil
}

func (m *Master) TaskCompleted(task *Task, reply *ExampleReply) error {
	mu.Lock()
	defer mu.Unlock()
	if task.TaskState != m.Phase || m.TaskMeta[task.Id].TaskStatus == Completed {
		return nil
	}
	m.TaskMeta[task.Id].TaskStatus = Completed
	go m.processTaskResult(task)
	return nil
}

func (m *Master) processTaskResult(task *Task) {
	mu.Lock()
	defer mu.Unlock()
	switch task.TaskState {
	case Map:
		// Collect the intermediates info.
		for reduceId, filePath := range task.InterMediates {
			m.InterMediates[reduceId] = append(m.InterMediates[reduceId], filePath)
		}
		if m.allTaskDone() {
			// Start Reduce phase after all Map finish.
			m.createReduceTask()
			m.Phase = Reduce
		}
	case Reduce:
		if m.allTaskDone() {
			m.Phase = Exit
		}
	}
}

func (m *Master) allTaskDone() bool {
	for _, task := range m.TaskMeta {
		if task.TaskStatus != Completed {
			return false
		}
	}
	return true
}
