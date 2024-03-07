package recorder

type Message interface {
	Type() string
	Id() string
	Data() interface{}
}

type MessageReplay struct {
	incoming chan Message

	msgs map[string][]Message
}

func NewMessageReplay() *MessageReplay {
	msgreplay := &MessageReplay{
		incoming: make(chan Message),
		msgs:     make(map[string][]Message),
	}
	go msgreplay.Process()
	return msgreplay
}

func (p *MessageReplay) Process() {
	//// dequeu the messages and put into the map
	for {
		msg := <-p.incoming
		if _, ok := p.msgs[msg.Id()]; !ok {
			//// create a new list
			p.msgs[msg.Id()] = make([]Message, 0)
		}
		p.msgs[msg.Id()] = append(p.msgs[msg.Id()], msg)
	}
}

func (p *MessageReplay) AddMessage(msg Message) {
	p.incoming <- msg
}

func (p *MessageReplay) Replay(id string) []Message {
	return p.msgs[id]
}
