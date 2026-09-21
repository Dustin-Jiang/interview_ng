package model

import "time"

// Bid 捡漏阶段某部门对某候选人的出价（candidate_id + department_id 唯一）。
// 出价对其他部门保密：HTTP 仅返回本部门出价，事件载荷不携带金额。
type Bid struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	CandidateID  uint64    `gorm:"uniqueIndex:idx_bid_candidate_department" json:"candidate_id"`
	DepartmentID uint64    `gorm:"uniqueIndex:idx_bid_candidate_department" json:"department_id"`
	Amount       int       `gorm:"not null" json:"amount"` // 出价额（预算点数）
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LeftoverMinBudget 捡漏部门预算下限。
const LeftoverMinBudget = 500

// LeftoverBidUnit 预算点数：每位缺口折算的出价单位。
const LeftoverBidUnit = 100

// LeftoverBudget 计算部门捡漏预算：max(500, (预期人数 - 已确认录取人数) * 100)。
func LeftoverBudget(expectedCount, admittedCount int) int {
	budget := (expectedCount - admittedCount) * LeftoverBidUnit
	if budget < LeftoverMinBudget {
		return LeftoverMinBudget
	}
	return budget
}

// DepartmentLeftover 部门捡漏预算概览。默认 Spent/Remaining 仅对本部门成员可见（出价保密）；
// 持 candidates.browse_all 的管理端请求中所有部门均公开。
type DepartmentLeftover struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	ExpectedCount int    `json:"expected_count"`
	AdmittedCount int    `json:"admitted_count"`
	Budget        int    `json:"budget"`
	Spent         *int   `json:"spent"`
	Remaining     *int   `json:"remaining"`
}

// MyLeftover 当前用户本部门的捡漏预算视图。
type MyLeftover struct {
	DepartmentID uint64 `json:"department_id"`
	Budget       int    `json:"budget"`
	Spent        int    `json:"spent"`
	Remaining    int    `json:"remaining"`
}

// LeftoverOverview 捡漏阶段总览（GET /api/leftover）。
type LeftoverOverview struct {
	Phase       SystemPhase          `json:"phase"`
	Departments []DepartmentLeftover `json:"departments"`
	My          *MyLeftover          `json:"my"`
}

// LeftoverResult 已结算候选人的录取结果（赢家 = 最高出价部门，全员可见）。
type LeftoverResult struct {
	CandidateID  uint64    `json:"candidate_id"`
	DepartmentID uint64    `json:"department_id"`
	Amount       int       `json:"amount"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LeftoverFinalResult 结算阶段最终录取结果（GET /api/leftover/projections，只读计算不落库）。
// 赢家 = 最高出价部门，同额取先出价者（bid id 更小）。
type LeftoverFinalResult struct {
	CandidateID  uint64 `json:"candidate_id"`
	DepartmentID uint64 `json:"department_id"` // 赢家部门
	Amount       int    `json:"amount"`        // 成交金额（赢家出价）
	Resolved     bool   `json:"resolved"`      // 是否已正式结算落库（存在 admitted 录取记录）
}
