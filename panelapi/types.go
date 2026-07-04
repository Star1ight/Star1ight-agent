package panelapi

type User struct {
	ID         int    `json:"id"`
	UUID       string `json:"uuid,omitempty"`
	Password   string `json:"password,omitempty"`
	Name       string `json:"name,omitempty"`
	SpeedLimit int    `json:"speed_limit,omitempty"`
}

type UserList struct {
	Users []User `json:"users"`
	Data  []User `json:"data,omitempty"`
}

type PushRequest map[int][]int64

type UsagePair struct {
	Total uint64
	Used  uint64
}

type NetSpeed struct {
	InSpeed  float64
	OutSpeed float64
}

type TrafficTotals struct {
	Up   uint64
	Down uint64
}

type MachineStatus struct {
	CPU          float64
	Mem          UsagePair
	Swap         UsagePair
	Disk         UsagePair
	Net          *NetSpeed
	Traffic      *TrafficTotals
	AgentVersion string
}
