package proplet

import "time"

const aliveTimeout = 10 * time.Second

type Proplet struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	TaskCount        uint64      `json:"task_count"`
	Alive            bool        `json:"alive"`
	AliveHistory     []time.Time `json:"alive_history"`
	Description      string      `json:"description,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	Location         string      `json:"location,omitempty"`
	IP               string      `json:"ip,omitempty"`
	Environment      string      `json:"environment,omitempty"`
	OS               string      `json:"os,omitempty"`
	Hostname         string      `json:"hostname,omitempty"`
	CPUArch          string      `json:"cpu_arch,omitempty"`
	TotalMemoryBytes uint64      `json:"total_memory_bytes,omitempty"`
	PropletVersion   string      `json:"proplet_version,omitempty"`
	WasmRuntime      string      `json:"wasm_runtime,omitempty"`
}

func (p *Proplet) SetAlive() {
	if len(p.AliveHistory) > 0 {
		lastAlive := p.AliveHistory[len(p.AliveHistory)-1]
		if time.Since(lastAlive) <= aliveTimeout {
			p.Alive = true

			return
		}
	}
	p.Alive = false
}

type PropletPage struct {
	Offset   uint64    `json:"offset"`
	Limit    uint64    `json:"limit"`
	Total    uint64    `json:"total"`
	Proplets []Proplet `json:"proplets"`
}

type ChunkPayload struct {
	AppName     string `json:"app_name"`
	ChunkIdx    int    `json:"chunk_idx"`
	TotalChunks int    `json:"total_chunks"`
	Data        []byte `json:"data"`
}
