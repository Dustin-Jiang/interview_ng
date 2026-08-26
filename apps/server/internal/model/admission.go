package model

import "time"

// AdmissionStatus 录取决定状态（录取阶段使用）：待定 / 录取 / 放弃。
// 按部门分别记录：候选人无固定归属部门，各部门可各自记录对同一候选人的录取决定。
type AdmissionStatus string

const (
	AdmissionPending   AdmissionStatus = "pending"   // 待定
	AdmissionAdmitted  AdmissionStatus = "admitted"  // 录取
	AdmissionWithdrawn AdmissionStatus = "withdrawn" // 放弃
)

// ValidAdmissionStatus 返回全部合法的录取决定状态。
func ValidAdmissionStatus() []AdmissionStatus {
	return []AdmissionStatus{AdmissionPending, AdmissionAdmitted, AdmissionWithdrawn}
}

// CandidateAdmission 某部门对某候选人的录取决定（candidate_id + department_id 唯一）。
// 候选人无固定部门，各部门可各自记录；默认只能浏览本部门录取状态，
// 持 candidates.browse_all 权限者可跨部门浏览全部部门的录取决定。
type CandidateAdmission struct {
	ID           uint64          `gorm:"primaryKey" json:"id"`
	CandidateID  uint64          `gorm:"uniqueIndex:idx_candidate_department" json:"candidate_id"`
	DepartmentID uint64          `gorm:"uniqueIndex:idx_candidate_department" json:"department_id"`
	Status       AdmissionStatus `gorm:"size:32" json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
