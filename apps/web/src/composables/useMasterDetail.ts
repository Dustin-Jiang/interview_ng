/**
 * useMasterDetail —— 主从（名册 ↔ 详情）两段式布局状态。
 * 桌面端恒为左右分栏，`showDetail` 只影响 lg 以下的移动端切换：
 * 选中候选人后进入详情，返回按钮退回名册。
 */
import { ref, type Ref } from 'vue'

export interface UseMasterDetail {
  /** 移动端是否处于详情态（选中候选人后置真）。 */
  readonly showDetail: Ref<boolean>
  openDetail: () => void
  closeDetail: () => void
}

export function useMasterDetail(): UseMasterDetail {
  const showDetail = ref(false)

  return {
    showDetail,
    openDetail: () => {
      showDetail.value = true
    },
    closeDetail: () => {
      showDetail.value = false
    },
  }
}
