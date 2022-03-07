# MIT 6.824 -- MapReduce

本文将讲解`MIT`分布式系统课程`6.824`的第一个`Lab`(mapreduce)的实验。

## 理论篇

这是2020年的分布式系统的实验。

在这个实验中，你将会构建一个 MapReduce System。你将会实现一个调用应用的 Map() 和 Reduce() 函数同时进行读/写文件的 worker 进程，和一个负责任务分发和处理异常 Worker 进程的 Coordinator 进程。你将会构建一个和提供的 MapReduce 论文类似的一个系统（Note：这个实验用 “Corrdinator” 指代论文中的 “Master” 角色）

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/mapreduce/mapreduce架构.png)



## 实验篇

### 1. 要求

您的工作是实现一个分布式MapReduce，它由两个程序（coordinator 和worker程序）组成。 只有一个 coordinator 程序进程，一个或多个worker进程并行执行。 在真实的系统中，工作人员将在一堆不同的机器上运行，但是对于本实验，您将全部在单个机器上运行。 workers 将通过 RPC 与coordinator 通信。 每个worker 进程都会向 coordinator 询问任务，从一个或多个文件读取任务的输入，执行任务，并将任务的输出写入一个或多个文件。 coordinator 应注意 worker是否在合理的时间内是否完成其任务（在本实验中，使用10秒），并将同一任务交给另一位不同的worker。

您应该将实现放在`mr/coordinator.go`，`mr/worker.go`和 `mr/rpc.go`中。

### 2. 前提知识

需要了解一下前提的知识

#### 2.1 利用unix进行进程间的通信

> **Unix domain socket** 或者 **IPC socket**是一种终端，可以使同一台[操作系统](https://zh.m.wikipedia.org/wiki/操作系统)上的两个或多个[进程](https://zh.m.wikipedia.org/wiki/进程)进行数据通信。与[管道](https://zh.m.wikipedia.org/wiki/管道_(Unix))相比，Unix domain sockets 既可以使用[字节流](https://zh.m.wikipedia.org/wiki/字節流)，又可以使用数据队列，而管道通信则只能使用[字节流](https://zh.m.wikipedia.org/wiki/字節流)。Unix domain sockets的接口和Internet socket很像，但它不使用网络底层协议来通信。Unix domain socket 的功能是[POSIX](https://zh.m.wikipedia.org/wiki/POSIX)操作系统里的一种组件。
>
> Unix domain sockets 使用系统文件的地址来作为自己的身份。它可以被系统进程引用。所以两个进程可以同时打开一个Unix domain sockets来进行通信。不过这种通信方式是发生在系统内核里而不会在网络里传播。

参考下面的文献，可以更好的了解：

- [Unix domain socket 跨进程通信](https://niconiconi.fun/2018/10/21/unix-domain-socket/)

- [Unix域套接字](https://zh.m.wikipedia.org/wiki/Unix%E5%9F%9F%E5%A5%97%E6%8E%A5%E5%AD%97)

代码示例：

```go
// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the master.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func masterSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}

//
// start a thread that listens for RPCs from worker.go
//
func (m *Master) server() {
	rpc.Register(m)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := masterSock()
    // 如果文件已存在，则需要移除，否者会监听失败
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
    // 启动HTTP服务
	go http.Serve(l, nil)
}
```

#### 2.2 RPC远程过程调用

> RPC是Remote Procedure Call，其中文意思就是 远程过程调用，可以理解成 一台主机上的进程调用另一台主机的进程服务，由一方为其它若干个主机提供服务。从表面上看非常类似于http API，RPC的目的可以屏蔽不同语言之间的关联，最大程度上进行解耦，调用方不需要知道服务方是用什么语言编写和其实现，只要知道服务方的RPC对外服务就行。其本质就是进程间的一种通信方式，可以是本机也可以是不同主机。

参考下面的文献，可以更好的了解：

- [RPC - 地鼠文档 (topgoer.cn)](https://www.topgoer.cn/docs/golang/chapter16-4)

代码示例：

```go
package main

import (
    "log"
    "net/http"
    "net/rpc"
)

// 例题：golang实现RPC程序，实现求矩形面积和周长

type Params struct {
    Width, Height int
}

type Rect struct{}

// RPC服务端方法，求矩形面积
func (r *Rect) Area(p Params, ret *int) error {
    *ret = p.Height * p.Width
    return nil
}

// 周长
func (r *Rect) Perimeter(p Params, ret *int) error {
    *ret = (p.Height + p.Width) * 2
    return nil
}

// 主函数
func main() {
    // 1.注册服务
    rect := new(Rect)
    // 注册一个rect的服务
    rpc.Register(rect)
    // 2.服务处理绑定到http协议上
    rpc.HandleHTTP()
    // 3.监听服务
    err := http.ListenAndServe(":8000", nil)
    if err != nil {
        log.Panicln(err)
    }
}
```

### 3. 设计思路

程序的设计思路是：

- 先设计`master`和`worker`进程之间的任务分配机制，也就是`worker`如何向`master`要任务，`master`又如何选择和分配任务。
- 再写`worker`内的`map`和`reduce` 的处理函数。

首先，为了简单起见，一个`file`即表示一个`map`任务，即，有多少`file`就有多少的`map`任务。

```go
type TaskStat struct {
	beginTime time.Time
	fileName  string       // 文件名，用于map程序读取文件
	fileIndex int          // 是map worker的编号，即文件的编号,为了之后方便存储
	partIndex int          // 是reduce worker的编号，为了之后方便存储
	nReduce   int          // 是map之后分成多少份的reudce，即reduce 程序的个数
	nFiles    int          // 是map 程序的总数，即，文件的总数
}
```

其次，master和worker之间 ，是通过rpc进行通信的。其主要功能有：

- work需要不断的向master发送请求，向master要任务

- worker通知master，如果是map任务完成，则master将map任务移除
- 利用unix，开启端口监听，用于进程间通信

## 4. 测试脚本



#### `timeout`命令

```shell
chui@cchui-virtual-machine:~$ timeout --help
用法：timeout [选项] 停留时间 命令 [参数]...
　或：timeout 选项
运行指定命令，在指定的停留时间后若该命令仍在运行则将其中止。

必选参数对长短选项同时适用。
      --preserve-status
                 退出时返回值与所运行命令的返回值保持相同，即使命令超时也
                   这样设置
      --foreground
                 当 timeout 不是直接从 shell 命令行开始运行时，允许所运行的
                   命令从 TTY 读取输入并获取 TTY 信号；在此模式下，所运行的
                   命令的子进程不会受超时的影响
  -k, --kill-after=持续时间
                 如果所运行命令在初始信号发出后再经过所指定持续时间以后仍然
                   在运行，则对其发送 KILL 信号
  -s, --signal=信号
                 指定超时发生时要发送的信号；
                   所指定的信号可以是如“HUP”这样的名称，或是一个数字；
                   请参见“kill -l”以获取可用信号列表
  -v, --verbose  对任何超时后发送的信号，向标准错误输出诊断信息
      --help		显示此帮助信息并退出
      --version		显示版本信息并退出
```



#### `crand.Int`

一个密码安全的伪随机数生成器



## 讨论

### 1. 实验缺陷

- 没有对输入的文件数据进行数据分割，而是直接一个`file`文件一个`map`任务











## 学习资料

- **[【MIT 6.824 Distributed Systems Spring 2020 分布式系统 中文翻译版合集】](https://www.bilibili.com/video/BV1x7411M7Sf?p=1)**
- **[课程表](https://pdos.csail.mit.edu/6.824/schedule.html)**

- **[课程的实验说明](https://pdos.csail.mit.edu/6.824/labs/lab-raft.html)**

- [**MIT 官方实验说明**](https://pdos.csail.mit.edu/6.824/labs/lab-mr.html)

- [**mapreduce论文翻译**](https://zhuanlan.zhihu.com/p/141657364)

- [**mapreduce原文**]([https://pdos.csail.mit.edu/6.824/papers/mapreduce.pdf)

- [**6.824 Lab1 MapReduce 中文翻译**](https://blog.buckbit.top/2021/04/15/labs-6824-mapreduce-translate)