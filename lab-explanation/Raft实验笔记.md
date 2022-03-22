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

#### 2.1 Leader选举





#### 2.2 日志复制







#### 2.3 安全机制





## 实验篇







## 测试





## 讨论
