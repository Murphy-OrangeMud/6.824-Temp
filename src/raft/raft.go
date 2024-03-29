package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "sync/atomic"
import "../labrpc"

import "math/rand"
import "time"

import "bytes"
import "../labgob"

import "fmt"
import "sort"

type State int

const (
	Follower = 1
	Candidate = 2
	Leader = 3
)

type Log struct {
	Command interface{}
	Index int
	Term int
}

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type TimeOut struct {
	Heartbeat  chan int
	InElection chan int
	OutOfElection chan int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain
	state int
	currentTerm int
	logs []Log
	getVotes int
	voteCandidate int
	electionTimeout int
	applyCh chan ApplyMsg

	// for leader
	nextIndex []int
	heartbeatInterval int
	matchIndex []int
	commitIndex int
	lastApplied int

	timeout TimeOut
	applyCond *sync.Cond
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	
	term = rf.currentTerm
	isleader = (rf.state == Leader)

	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.logs)
	e.Encode(rf.voteCandidate)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}


//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)

	var currentTerm int
	var logs []Log
	var voteCandidate int

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if d.Decode(&currentTerm) != nil {
		fmt.Println("Decode failed: currentTerm")
	} else {
		rf.currentTerm = currentTerm
	}

	if d.Decode(&logs) != nil {
		fmt.Println("Decode failed: logs")
	} else {
		rf.logs = logs
	}

	if d.Decode(&voteCandidate) != nil {
		fmt.Println("Decode failed: voteCandidate")
	} else {
		rf.voteCandidate = voteCandidate
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	CurrentTerm int
	Logs []Log
	ID int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	ReturnValue int
	CurrentTerm int
}

//
// example RequestVote RPC handler.
//
// rf: callee, requestVoteArgs: caller
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	if args.CurrentTerm < rf.currentTerm {
		reply.CurrentTerm = rf.currentTerm
		reply.ReturnValue = -1
		return
	} else if rf.state == Leader && rf.currentTerm == args.CurrentTerm {
		reply.CurrentTerm = -1
		reply.ReturnValue = -4
	} else if len(args.Logs) > 0 && len(rf.logs) > 0 && args.Logs[len(args.Logs) - 1].Term < rf.logs[len(rf.logs) - 1].Term {
		// not so update
		reply.CurrentTerm = -1
		reply.ReturnValue = -2
		return
	} else if len(args.Logs) > 0 && len(rf.logs) > 0 && args.Logs[len(args.Logs) - 1].Term == rf.logs[len(rf.logs) - 1].Term && len(args.Logs) < len(rf.logs) {
		// not so update
		reply.CurrentTerm = -1
		reply.ReturnValue = -2
		return
	} else if rf.voteCandidate != args.ID && rf.voteCandidate != -1 {
		reply.CurrentTerm = -1
		reply.ReturnValue = -3
		return
	} else {
		// go func() { rf.timeout.InElection <- 2; } ()
		rf.voteCandidate = args.ID
		rf.persist()
		reply.ReturnValue = 1
		return
	}
}

type AppendEntriesArgs struct {
	CurrentTerm int
	Logs []Log
	ID int
	NextIndex int
	LeaderCommitIndex int
}

type AppendEntriesReply struct {
	ReturnValue int
	CurrentTerm int
	NextIndex int
}

// rf: callee args: leader
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	reply.CurrentTerm = -1
	if args.CurrentTerm < rf.currentTerm {
		reply.ReturnValue = -1
		reply.CurrentTerm = rf.currentTerm
	} else {
		// reset timeout
		go func() { rf.timeout.OutOfElection <- 3; }()
		// lab 2a
		if rf.currentTerm < args.CurrentTerm {
			rf.mu.Lock()
			rf.currentTerm = args.CurrentTerm
			rf.persist()
			rf.mu.Unlock()
		}
		if rf.state == Candidate || rf.state == Leader {
			rf.mu.Lock()
			rf.state = Follower
			rf.voteCandidate = -1
			rf.getVotes = 0
			rf.persist()
			rf.mu.Unlock()
		}

		if len(args.Logs) == 0 {
			// heartbeat
			reply.ReturnValue = -2
			return
		}

		// lab 2b
		if args.NextIndex > 1 && args.NextIndex <= len(rf.logs) + 1 {
			nextIdx := args.NextIndex
			for nextIdx >= 2 && !(rf.logs[nextIdx - 2].Index == args.Logs[nextIdx - 2].Index && rf.logs[nextIdx - 2].Term == args.Logs[nextIdx - 2].Term) {
				nextIdx -= 1
			}
			rf.logs = rf.logs[:nextIdx - 1]
			for i := nextIdx - 1; i < len(args.Logs); i++ {
				rf.logs = append(rf.logs, args.Logs[i])
			}
			rf.persist()
			reply.NextIndex = len(args.Logs) + 1
			reply.ReturnValue = 1
		} else if args.NextIndex > len(rf.logs) + 1 {
			reply.NextIndex = len(rf.logs) + 1
			reply.ReturnValue = -1
		} else {
			// args.NextIndex == 1
			if len(args.Logs) > 0 {
				for i := 0; i < len(args.Logs); i++ {
					rf.logs = append(rf.logs, args.Logs[i])
				}
				rf.persist()
				reply.NextIndex = len(args.Logs) + 1
				reply.ReturnValue = 1
			}
		}

		if reply.ReturnValue == 1 && args.LeaderCommitIndex > rf.commitIndex {
			if args.LeaderCommitIndex < len(args.Logs) {
				rf.applyCond.L.Lock()
				rf.commitIndex = args.LeaderCommitIndex
				rf.applyCond.Broadcast()
				rf.applyCond.L.Unlock()
			} else {
				rf.applyCond.L.Lock()
				rf.commitIndex = len(args.Logs)
				rf.applyCond.Broadcast()
				rf.applyCond.L.Unlock()
			}
		}
	}
	// for debug
	//fmt.Println(rf.me, reply.NextIndex, reply.ReturnValue)
	//fmt.Printf("After Appending Entries, server %d's commitindex %d, applyindex %d, log: ", rf.me, rf.commitIndex, rf.lastApplied)
	//fmt.Println(rf.logs)
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	isLeader = (rf.state == Leader) && (!rf.killed())
	term = rf.currentTerm
	index = len(rf.logs) + 1

	if isLeader == false {
		return index, term, isLeader
	} else {
		rf.mu.Lock()
		rf.logs = append(rf.logs, Log{ Command: command, Index: len(rf.logs) + 1, Term: rf.currentTerm })
		rf.persist()
		rf.mu.Unlock()
		// for debug
		//fmt.Printf("Leader: %v, ", rf.me)
		//fmt.Println(rf.logs)

		return index, term, isLeader
	}
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

type mIndex struct {
	id int
	index int
}
type mIndices []mIndex

func (idx mIndices) Less(i, j int) bool {
	return idx[i].index > idx[j].index
}

func (idx mIndices) Len() int {
	return len(idx)
}

func (idx mIndices) Swap(i, j int) {
	idx[i], idx[j] = idx[j], idx[i]
}

func (rf *Raft) commit() {
	if rf.killed() || rf.state != Leader {
		return
	}
	// reinitialize after election
	rf.mu.Lock()
	for i, _ := range rf.peers {
		rf.nextIndex[i] = len(rf.logs) + 1
		rf.matchIndex[i] = 0
	}
	rf.mu.Unlock()
	for {
		time.Sleep(1 * time.Millisecond)
		if rf.killed() || rf.state != Leader {
			return
		}
		type CommitResult struct {
			id int
			ok bool
			reply AppendEntriesReply
		}
		commitResultChan := make(chan CommitResult)

		for i, _ := range rf.peers {
			go func (id int) {
				if id == rf.me {
					return
				}

				args := AppendEntriesArgs {
					CurrentTerm: rf.currentTerm,
					Logs: rf.logs,
					ID: rf.me,
					NextIndex: rf.nextIndex[id],
					LeaderCommitIndex: rf.commitIndex,
				}
				reply := AppendEntriesReply {}

				ok := rf.sendAppendEntries(id, &args, &reply)
				commitResultChan <- CommitResult {
					id: id,
					ok: ok,
					reply: reply,
				}

			} (i)
		}

		finishCount := 0
		for finishCount < len(rf.peers) - 1 && rf.state == Leader {
			select {
			case commitResult := <- commitResultChan:
				finishCount++
				if commitResult.ok == false {
					continue
				}
	
				if commitResult.reply.ReturnValue == -1 {
					if commitResult.reply.CurrentTerm != -1 {
						rf.mu.Lock()
						rf.currentTerm = commitResult.reply.CurrentTerm
						rf.persist()
						rf.state = Follower
						rf.mu.Unlock()
						return
					} else {
						rf.nextIndex[commitResult.id] = commitResult.reply.NextIndex
					}
				} else if commitResult.reply.ReturnValue == 1 {
					rf.mu.Lock()
					rf.nextIndex[commitResult.id] = commitResult.reply.NextIndex
					rf.matchIndex[commitResult.id] = rf.nextIndex[commitResult.id] - 1
					rf.mu.Unlock()
				}
			case <- time.After(((time.Duration) (rf.electionTimeout / 10)) * time.Millisecond):
				finishCount++
				break
			}
		}
		// sort interface to find median of matchIndex
		sortMatchIndex := mIndices{}
		for i := 0; i < len(rf.peers); i++ {
			if i == rf.me {
				continue
			}
			sortMatchIndex = append(sortMatchIndex, mIndex { id: i, index: rf.matchIndex[i] })
		}
		sort.Sort(sortMatchIndex)

		rf.applyCond.L.Lock()
		rf.commitIndex = sortMatchIndex[len(rf.peers) / 2 - 1].index // and leader
		rf.applyCond.Broadcast()
		rf.applyCond.L.Unlock()
 	}
}

// daemon thread
func (rf *Raft) run() {
	// go func() { rf.timeout.OutOfElection <- 3; } ()
	for {
		switch (rf.state) {
		case Follower:
			select {
			case <- rf.timeout.OutOfElection:
				continue;
			case <- time.After(((time.Duration) (rf.electionTimeout)) * time.Millisecond):
				rf.elect()
				if rf.state == -1 {
					return
				} else if rf.state == Leader {
					go rf.commit()
					break
				}
				go func() { rf.timeout.OutOfElection <- 3; } ()
				rf.mu.Lock()
				rf.voteCandidate = -1
				rf.getVotes = 0
				rf.persist()
				rf.mu.Unlock()
			}
		case Leader:
			select {
			case <- rf.timeout.Heartbeat:
				continue;
			case <- time.After(((time.Duration) (rf.heartbeatInterval)) * time.Millisecond):
				// heartbeat time
				go func() { rf.timeout.Heartbeat <- 1; } ()
				for i, _ := range rf.peers {
					go func(id int) {
						if id == rf.me {
							return
						}
						args := AppendEntriesArgs {
							CurrentTerm: rf.currentTerm, 
							Logs: []Log{},
							ID: rf.me,
							NextIndex: rf.nextIndex[id],
							LeaderCommitIndex: rf.commitIndex,
						}
						reply := AppendEntriesReply {}
						ok := rf.sendAppendEntries(id, &args, &reply)
						
						if ok == false {
							return 
						}
						if reply.ReturnValue == -1 && reply.CurrentTerm > rf.currentTerm {
							rf.mu.Lock()
							rf.state = Follower
							rf.currentTerm = reply.CurrentTerm
							rf.getVotes = 0
							rf.voteCandidate = -1
							rf.persist()
							rf.mu.Unlock()
							return
						}

					} (i)
					
				}
			}
		}
		time.Sleep(1 * time.Millisecond)
		if rf.killed() {
			return
		}
	}
}

func (rf *Raft) elect() {
	rf.mu.Lock()
	rf.currentTerm++
	rf.state = Candidate
	rf.getVotes = 1
	rf.voteCandidate = rf.me
	rf.persist()
	rf.mu.Unlock()

	go func() { rf.timeout.InElection <- 2; } ()

	for {
		if rf.killed() {
			rf.state = -1
			return
		}
		select {
		case <- rf.timeout.InElection:
			type VoteResult struct { 
				id int
				ok bool
				reply RequestVoteReply
			}
			voteResultChan := make(chan VoteResult)
			for i, _ := range rf.peers {
				go func(id int) {
					if id == rf.me {
						return
					}
					args := RequestVoteArgs {
						rf.currentTerm,
						rf.logs,
						rf.me,
					}
					reply := RequestVoteReply {}
					ok := rf.sendRequestVote(id, &args, &reply)
					voteResultChan <- VoteResult {id: id, ok: ok, reply: reply}
				} (i)
			}

			finishCnt := 0
			for finishCnt < len(rf.peers) - 1 {
				select {
				case result := <- voteResultChan:
					finishCnt++
					if finishCnt == len(rf.peers) - 1 {
						break
					}
					if result.ok == false {
						continue
					}
					if result.reply.ReturnValue == 1 {
						rf.getVotes++
						if rf.getVotes > len(rf.peers) / 2 {
							rf.mu.Lock()
							rf.state = Leader
							rf.voteCandidate = -1
							rf.getVotes = 0
							rf.persist()
							rf.mu.Unlock()
							return
						}
					} else if result.reply.CurrentTerm != -1 {
						rf.mu.Lock()
						rf.state = Follower
						rf.voteCandidate = -1
						rf.getVotes = 0
						rf.currentTerm = result.reply.CurrentTerm
						rf.persist()
						rf.mu.Unlock()
						return
					} else if result.reply.ReturnValue == -4 {
						rf.mu.Lock()
						rf.state = Follower
						rf.voteCandidate = -1
						rf.getVotes = 0
						rf.persist()
						rf.mu.Unlock()
						return
					}
				case <- time.After(((time.Duration) (rf.electionTimeout)) * time.Millisecond):
					rf.mu.Lock()
					rf.state = Follower
					rf.voteCandidate = -1
					rf.getVotes = 0
					rf.persist()
					rf.mu.Unlock()
					return
				}
			}
		case <- time.After(((time.Duration) (rf.electionTimeout)) * time.Millisecond):
			rf.mu.Lock()
			rf.state = Follower
			rf.voteCandidate = -1
			rf.getVotes = 0
			rf.persist()
			rf.mu.Unlock()
			return
		}
	}
}

func (rf *Raft) applyLog() {
	for !rf.killed() {
		rf.applyCond.L.Lock()
		for rf.commitIndex <= rf.lastApplied {
			rf.applyCond.Wait()
		}
		rf.lastApplied++
		rf.applyCond.L.Unlock()
		rf.applyCh <- ApplyMsg {
			CommandValid: true,
			Command: rf.logs[rf.lastApplied - 1].Command,
			CommandIndex: rf.logs[rf.lastApplied - 1].Index,
		}
	}
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	//rf.mu.Lock()
	rf.logs = []Log{}
	rf.nextIndex = []int{}
	rf.matchIndex = []int{}
	rf.currentTerm = 0
	rf.voteCandidate = -1
	rf.state = Follower
	rf.matchIndex = []int{}
	rf.getVotes = 0
	rf.heartbeatInterval = 100 // ms
	rf.applyCh = applyCh
	rf.lastApplied = 0
	rf.commitIndex = 0
	rf.applyCond = sync.NewCond( &sync.Mutex{} )
	
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex = append(rf.nextIndex, 1)
		rf.matchIndex = append(rf.matchIndex, 0)
	}
	//rf.mu.Unlock()

	rf.timeout.Heartbeat = make(chan int)
	rf.timeout.InElection = make(chan int)
	rf.timeout.OutOfElection = make(chan int)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	rf.electionTimeout = r.Intn(500) + 200 //ms

	// initialize from state persisted before a crash
	rf.readPersist(rf.persister.ReadRaftState())

	go rf.run()
	go rf.applyLog()

	return rf
}
