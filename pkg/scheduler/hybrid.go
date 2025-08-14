package scheduler

import (
	"fmt"

	"github.com/absmach/propeller/proplet"
	"github.com/absmach/propeller/task"
)

// HybridScheduler implements scheduling for both K8s and external proplets
type HybridScheduler struct {
	k8sScheduler      Scheduler
	externalScheduler Scheduler
}

// NewHybridScheduler creates a scheduler that can handle both K8s and external proplets
func NewHybridScheduler(k8sScheduler, externalScheduler Scheduler) *HybridScheduler {
	if k8sScheduler == nil {
		k8sScheduler = NewRoundRobinScheduler()
	}
	if externalScheduler == nil {
		externalScheduler = NewRoundRobinScheduler()
	}
	
	return &HybridScheduler{
		k8sScheduler:      k8sScheduler,
		externalScheduler: externalScheduler,
	}
}

// SelectProplet chooses the best proplet for a task considering both K8s and external proplets
func (h *HybridScheduler) SelectProplet(t task.Task, proplets []proplet.Proplet) (proplet.Proplet, error) {
	if len(proplets) == 0 {
		return proplet.Proplet{}, ErrNoPropletAvailable
	}
	
	// Separate proplets by type (this would require extending the proplet.Proplet struct)
	var k8sProplets []proplet.Proplet
	var externalProplets []proplet.Proplet
	
	for _, p := range proplets {
		// For now, we'll use a simple heuristic to identify proplet types
		// This could be enhanced by adding a Type field to the proplet.Proplet struct
		if isK8sProplet(p) {
			k8sProplets = append(k8sProplets, p)
		} else {
			externalProplets = append(externalProplets, p)
		}
	}
	
	// Check if task has preference for proplet type
	preference := getTaskPropletPreference(t)
	
	switch preference {
	case "k8s":
		if len(k8sProplets) > 0 {
			return h.k8sScheduler.SelectProplet(t, k8sProplets)
		}
		// Fallback to external if no K8s proplets available
		if len(externalProplets) > 0 {
			return h.externalScheduler.SelectProplet(t, externalProplets)
		}
		
	case "external":
		if len(externalProplets) > 0 {
			return h.externalScheduler.SelectProplet(t, externalProplets)
		}
		// Fallback to K8s if no external proplets available
		if len(k8sProplets) > 0 {
			return h.k8sScheduler.SelectProplet(t, k8sProplets)
		}
		
	default: // "any" or no preference
		// Use a weighted approach: prefer K8s for compute-intensive tasks, external for edge tasks
		selectedProplets := selectPropletsByTaskCharacteristics(t, k8sProplets, externalProplets)
		
		if len(selectedProplets) > 0 {
			if len(k8sProplets) > 0 && containsK8sProplets(selectedProplets, k8sProplets) {
				return h.k8sScheduler.SelectProplet(t, filterK8sProplets(selectedProplets, k8sProplets))
			}
			return h.externalScheduler.SelectProplet(t, filterExternalProplets(selectedProplets, externalProplets))
		}
	}
	
	return proplet.Proplet{}, ErrNoPropletAvailable
}

// isK8sProplet determines if a proplet is running in Kubernetes
func isK8sProplet(p proplet.Proplet) bool {
	// Simple heuristic: K8s proplets typically have names with UUID format
	// or contain certain patterns. This could be enhanced by adding metadata.
	// For now, we'll use a name pattern approach
	return len(p.ID) > 10 && (containsKubernetesPattern(p.Name) || containsKubernetesPattern(p.ID))
}

// containsKubernetesPattern checks if the name follows Kubernetes naming patterns
func containsKubernetesPattern(name string) bool {
	// Look for patterns typical in K8s generated names
	return len(name) > 20 || // Long names typical of K8s generated names
		   containsPattern(name, "proplet-") // Our deployment naming convention
}

// containsPattern is a simple pattern matcher
func containsPattern(s, pattern string) bool {
	return len(s) >= len(pattern) && s[:len(pattern)] == pattern
}

// getTaskPropletPreference extracts proplet preference from task metadata
func getTaskPropletPreference(t task.Task) string {
	// This would be enhanced to read from task metadata/annotations
	// For now, return "any" as default
	return "any"
}

// selectPropletsByTaskCharacteristics chooses proplets based on task characteristics
func selectPropletsByTaskCharacteristics(t task.Task, k8sProplets, externalProplets []proplet.Proplet) []proplet.Proplet {
	// Simple heuristics for task placement:
	
	// 1. If task name suggests it's for edge/IoT, prefer external proplets
	if isEdgeTask(t) && len(externalProplets) > 0 {
		return externalProplets
	}
	
	// 2. If task suggests heavy computation, prefer K8s proplets
	if isComputeIntensiveTask(t) && len(k8sProplets) > 0 {
		return k8sProplets
	}
	
	// 3. Default: combine both and let load balancing decide
	combined := make([]proplet.Proplet, 0, len(k8sProplets)+len(externalProplets))
	combined = append(combined, k8sProplets...)
	combined = append(combined, externalProplets...)
	
	return combined
}

// isEdgeTask determines if a task is suitable for edge execution
func isEdgeTask(t task.Task) bool {
	taskName := t.Name
	// Simple keyword-based detection
	edgeKeywords := []string{"sensor", "iot", "edge", "device", "monitor", "collect"}
	
	for _, keyword := range edgeKeywords {
		if containsPattern(taskName, keyword) {
			return true
		}
	}
	
	return false
}

// isComputeIntensiveTask determines if a task requires significant compute resources
func isComputeIntensiveTask(t task.Task) bool {
	taskName := t.Name
	// Simple keyword-based detection
	computeKeywords := []string{"ml", "ai", "compute", "process", "analyze", "transform", "batch"}
	
	for _, keyword := range computeKeywords {
		if containsPattern(taskName, keyword) {
			return true
		}
	}
	
	return false
}

// Helper functions for filtering proplets
func containsK8sProplets(selected, k8s []proplet.Proplet) bool {
	for _, s := range selected {
		for _, k := range k8s {
			if s.ID == k.ID {
				return true
			}
		}
	}
	return false
}

func filterK8sProplets(selected, k8s []proplet.Proplet) []proplet.Proplet {
	var filtered []proplet.Proplet
	for _, s := range selected {
		for _, k := range k8s {
			if s.ID == k.ID {
				filtered = append(filtered, s)
				break
			}
		}
	}
	return filtered
}

func filterExternalProplets(selected, external []proplet.Proplet) []proplet.Proplet {
	var filtered []proplet.Proplet
	for _, s := range selected {
		for _, e := range external {
			if s.ID == e.ID {
				filtered = append(filtered, s)
				break
			}
		}
	}
	return filtered
}

// ResourceAwareScheduler implements scheduling based on resource requirements
type ResourceAwareScheduler struct {
	fallback Scheduler
}

// NewResourceAwareScheduler creates a scheduler that considers resource constraints
func NewResourceAwareScheduler() *ResourceAwareScheduler {
	return &ResourceAwareScheduler{
		fallback: NewRoundRobinScheduler(),
	}
}

// SelectProplet chooses proplets based on available resources
func (r *ResourceAwareScheduler) SelectProplet(t task.Task, proplets []proplet.Proplet) (proplet.Proplet, error) {
	if len(proplets) == 0 {
		return proplet.Proplet{}, ErrNoPropletAvailable
	}
	
	// Filter proplets that have sufficient resources
	// This is a simplified version - in a real implementation, we'd check actual resource availability
	availableProplets := make([]proplet.Proplet, 0)
	
	for _, p := range proplets {
		if hasInsufficientResources(p, t) {
			continue
		}
		availableProplets = append(availableProplets, p)
	}
	
	if len(availableProplets) == 0 {
		return proplet.Proplet{}, fmt.Errorf("no proplets have sufficient resources for task %s", t.Name)
	}
	
	// Use fallback scheduler for final selection among suitable proplets
	return r.fallback.SelectProplet(t, availableProplets)
}

// hasInsufficientResources checks if proplet lacks resources for the task
func hasInsufficientResources(p proplet.Proplet, t task.Task) bool {
	// Simplified resource checking
	// In real implementation, this would check CPU, memory, and custom resource requirements
	
	// Basic heuristic: if proplet is overloaded (too many tasks), skip it
	maxTasksPerProplet := uint64(5) // Configurable threshold
	
	return p.TaskCount >= maxTasksPerProplet
}