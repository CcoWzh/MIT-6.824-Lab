# Raft实验笔记

本文将讲解`MIT`分布式系统课程`6.824`的第二个`Lab`(Raft)的实验。

Raft是分布式系统中非常有名的共识算法（所谓共识算法，是指在容错的分布式系统中，如何对一个值，或者状态，在各节点间达成一致性的过程），业界已经有了很好的实现了，比如[etcd](https://github.com/coreos/etcd)、[TiKV](https://github.com/pingcap/tikv) 等，相比于Paxos算法，Raft具有更好的可理解性，这也是他广泛使用的原因之一。

实现一个简易的Raft算法是很有意义的，这可以帮助我们更好的理解Raft的原理，这个过程中也会学习到很多编程技巧（如，怎么应对并发、怎么进行分布式编程、怎么调试BUG等等），利于提高我们的编程能力。更进一步的，对于分布式共识方面的研究生，在设计共识算法时，可以更好的思考如何设计一个优美的共识算法。

话不多说，开干！

## 原理篇

### 1. 学习资料

下面是学习Raft时，一些非常好的资料：

- [The Raft Consensus Algorithm](https://raft.github.io/) Raft官方资料，挺全面的
- &#x1F34E;[Raft演示动画](http://thesecretlivesofdata.com/raft/)  Raft的演示动画
- &#x1F34E;[Raft 作者亲自出的 Raft 试题](https://ongardie.net/static/raft/userstudy/quizzes.html) Raft 作者亲自出的 Raft 试题，你能做对几道？
- [Raft 作者亲自出的 Raft 试题(答案)](https://tangwz.com/post/raft-exam/)  上面链接的答案
- [分布式一致性算法：Raft 算法（论文翻译）](https://www.cnblogs.com/linbingdong/p/6442673.html) 论文翻译

### 2. 论文理解

下面是我在读Raft论文时做的阅读思维导图：

<img src=https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft一致性算法.png#pic_center width=80% />

Raft论文的主要精华是：领导者（Leader）选举、日志复制、安全机制三大块。具体的，有：

#### 2.1 基本概念

需要搞清楚Raft中的领导者选举、日志复制过程和其他补充说明，首先需要知道Raft中的一些基本的概念，比如角色、任期、假设前提、容错等等。

##### 前提假设

Raft是非拜占庭容错，即，节点不会作恶（不会故意偏离协议，不会发送错误的消息等）。

##### 角色

Raft中一个有3种角色：领导者（`leader`）、跟随者（`follower`）或者候选者（`candidate`），其中：

- `Leader`：负责处理所有的客户端请求，并负责日志到所有的节点上；
- `Follower`：不会发送任何请求，只是简单的响应来自 leader 和 candidate 的请求；
- `Candidate`：在`leader`失效时，由`follower`状态转变为`candidate`，竞争成为`leader`。

也就是说，在正常情况下，集群中只有一个 `leader` 节点，其他的节点全部都是 `follower` 。其角色转换关系如下图所示：

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-选举过程-4.png)

##### 任期

Raft 将时间分割成任意长度的任期（term），如下图所示：

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-任期.png)

- 任期开始：任期开始时，一个或多个节点由`follower`变为`candidate`，试图竞争成为领导者。如果一个候选人赢得了选举，它就会在该任期的剩余时间担任领导人。
- 任期结束：当前任期的领导者失效（即，没有及时发送心跳给其他节点，一段时间后即认为节点失效）时，任期结束，其他节点自动竞争成为领导者

Raft 算法保证在给定的一个任期最多只有一个领导人。

##### 容错

Raft一般认为，应该设定`2f+1`个节点（`f`为容错量），是合理的。因为是要获得投票数量为总节点数量的一半以上。

#### 2.2 Leader选举

有了上面的基础，即可开始描述Raft中，`Leader`是如何选举的了。

Raft 使用**心跳机制**来触发 leader 选举。

> 当网络初始化时，所有的节点都是以`Follower `状态启动的。而且，一个服务器节点只要能从` leader` 或 `candidate` 处接收到有效的 RPC 就一直保持 `follower` 状态。`Leader `会周期性地向所有 `follower` 发送心跳来维持自己的地位。

如果一个` follower `在一段选举超时时间内没有接收到任何消息，它就假设系统中没有可用的 `leader`，然后开始进行选举以选出新的 leader 。理想情况下，整个选举过程如下图所示：

![leader选举过程](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-选举过程-1.png)

当然，过程可能不会这么理想，因为成为候选者的节点不会只有一个，**他们之间可能存在竞争关系**。

Candidate 会一直保持候选状态直到以下三件事情之一发生：

- 它自己赢得了这次的选举（收到过半的投票）；
- 其他的服务器节点成为 leader ；
-  一段时间之后没有任何获胜者。

试想一下这样的场景：

有多个`follower` 同时成为 `candidate`，则选票可能会被瓜分，导致本轮没有一个节点收集到过半的选票。如下图所示：

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-选举过程-2.png)

当这种情况发生时，每一个候选人都会超时，然后通过增加当前任期号来开始一轮新的选举。然而，如果没有其他机制的话，该情况可能会无限重复。

Raft 算法使用随机选举超时时间的方法来确保很少发生选票瓜分的情况，就算发生也能很快地解决。为了阻止选票一开始就被瓜分，选举超时时间是从一个固定的区间（例如 150-300 毫秒）随机选择。这样可以把服务器都分散开以至于在大多数情况下只有一个服务器会选举超时；然后该服务器赢得选举并在其他服务器超时之前发送心跳。同样的机制被用来解决选票被瓜分的情况。如下图所示。

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-选举过程-3.png)



每个` candidate `在开始一次选举的时候会重置一个随机的选举超时时间，然后一直等待直到选举超时；这样减小了在新的选举中再次发生选票瓜分情况的可能性。

> 当` candidate `收到同任期的数据追加请求或者更高任期的请求，` candidate `会转变为`follower`跟随对方。

#### 2.3 日志复制

在 Raft 中，日志条目（log entries）只从 leader 流向其他服务器。在正常情况下，日志的复制过程如下图所示：

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-日志复制-3.png)

##### 异常情况

Leader或者Follower都可能会宕机崩溃，从而导致系统中各节点间保存的日志不一致。如下图所示：

![](https://cdn.jsdelivr.net/gh/CcoWzh/CDNimage/etcd/raft/Raft-日志复制-2.png)

主要是会存在以下三种情况：

**（1）缺失日志**

`Follower`缺失了一些`Leader`上存在的日志条目。这可能是由于`follower`宕机失效，之后又重新启动了。

**（2）存有`Leader`不存在的日志**

可能`Follower`也会存在当前`Leader`不存在的日志条目。

这可能是由于之前的`Leader`仅仅将`AppendEntries RPC`消息发送到一部分`Follower`就崩溃掉，然后新的当选`Leader`的服务器恰好是没有收到该`AppendEntries RPC`消息的服务器。

**（3）以上两种均有**

如，出现`F`的情况可能是这样的：该节点是第2个任期的领导者，在其日志中添加了若干个日志条目，然后在提交其中任何一个之前崩溃；之后很快重新启动，成为第3个任期的领导者，并在其日志中添加了更多的条目；在提交第2项或第3项中的任何一项之前，服务器再次崩溃，并在接下来的几个任期内保持关闭（宕机）状态，则会出现这样的情况。

##### 如何保持日志一致

首先，需要聊聊Raft中的一致性检查：

> 一致性检查

`Leader`节点在发送 `AppendEntries RPC `的时候，会将**前一个日志条目的索引位置和任期号**包含在里面。即：

```go
type AppendEntriesRPC struct{
    term            int  // 领导者的任期
    leaderId        int  // 领导者的ID
    prevLogIndex    int  // 紧邻新日志条目之前的那个日志条目的索引
    prevLogTerm     int  // 紧邻新日志条目之前的那个日志条目的任期
    entries[]       int  // 需要被保存的日志条目
    leaderCommit    int  // 领导者的已知已提交的最高的日志条目的索引
}
```

如果 `follower` 在它的日志中找不到包含相同索引位置和任期号的条目，那么他就会拒绝该新的日志条目。返回的结果为：

```go
type Reault struct{
    term     int   // 节点当前的任期，用于leader自我更新
    success  bool  // 当节点的prevLogIndex、prevLogTerm都检查通过时，返回ture
}
```

一致性检查就像一个归纳步骤：一开始空的日志状态肯定是满足 Log Matching Property（日志匹配特性）的，然后一致性检查保证了日志扩展时的日志匹配特性。因此，每当` AppendEntries RPC `返回成功时，`leader` 就知道 `follower` 的日志一定和自己相同（从第一个日志条目到最新条目）。

这也维护了Raft中的日志匹配特性：

- 如果不同日志中的两个条目拥有相同的索引和任期号，那么他们存储了相同的指令。
- 如果不同日志中的两个条目拥有相同的索引和任期号，那么他们之前的所有日志条目也都相同。

> 如何保持日志的一致性？

**在Raft 算法中，leader 通过强制 follower 复制它的日志来解决不一致的问题。**

> 具体怎么做？

要使得 `follower` 的日志跟自己一致，`leader `必须找到两者达成一致的最大的日志条目（索引最大），删除 `follower `日志中从那个点之后的所有日志条目，并且将自己从那个点之后的所有日志条目发送给 `follower` 。具体的做法为：

- 当发生日志冲突时，`Follower`将会拒绝由`Leader`发送的`AppendEntries RPC`消息，并返回一个响应消息告知`Leader`日志发生了冲突。
- `Leader`为每一个`Follower`维护一个`nextIndex`值。该值用于确定需要发送给该`Follower`的下一条日志的位置索引。(该值在当前服务器成功当选`Leader`后会重置为本地日志的最后一条索引号+1)
- 当`Leader`了解到日志发生冲突之后，便递减`nextIndex`值。并重新发送`AppendEntries RPC`到该`Follower`。并不断重复这个过程，一直到`Follower`接受该消息。
- 一旦`Follower`接受了`AppendEntries RPC`消息，`Leader`则根据`nextIndex`值可以确定发生冲突的位置，从而强迫`Follower`的日志重复自己的日志以解决冲突问题。

> 思考一个这样的问题：

***一个 follower 可能会进入不可用状态，在此期间，leader 可能提交了若干的日志条目，然后这个 follower 可能会被选举为 leader 并且用新的日志条目覆盖这些日志条目；结果，不同的状态机可能会执行不同的指令序列。***

也就是说，以上的做法，当一个新的`Follower`成为了`Leader`之后，他的日志可能就会覆盖之前存在的、已经被大多数节点存储的日志。这个是很危险的！！

解决解决这个问题，Raft对节点选举上做了限制。

> 节点选举限制

Raft 使用投票的方式来阻止 candidate 赢得选举除非该 candidate 包含了所有已经提交的日志条目。

候选人为了赢得选举必须与集群中的过半节点通信，如果 candidate 的日志至少和过半的服务器节点一样新，那么他一定包含了所有已经提交的日志条目。

来看其`RequestVote RPC` 的结构：

```go
type RequestVote RPC struct{
    term            int  // 新的任期
    candidateId     int  // 候选者的ID
    lastLogIndex    int  // 候选者最后一个日志条目的索引
    lastLogTerm     int  // 候选者最后一次登录的期限
}
```

在投票前，应该验证以下两点：

- 是否为`term < currentTerm`;
- 候选人的日志**至少**与接收者的日志一样新

`RequestVote RPC` 执行了这样的限制： RPC 中包含了 candidate 的日志信息，如果投票者自己的日志比 candidate 的还新，它会拒绝掉该投票请求。

> 什么叫最新呢？

Raft 通过比较两份日志中最后一条日志条目的索引值和任期号来定义谁的日志比较新。

- 如果两份日志最后条目的任期号不同，那么任期号大的日志更新。
- 如果两份日志最后条目的任期号相同，那么日志较长的那个更新。



## 实验篇







## 测试





## 讨论
