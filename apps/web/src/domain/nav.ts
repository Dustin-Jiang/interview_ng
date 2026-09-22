/**
 * 顶部导航高亮判定（纯函数，无 Vue 依赖）。
 *
 * 页面路由是 RESTful 的：集合页与它的条目页/子路由是**兄弟路由**，各有自己的 name
 * （`/candidates` = `candidates`，`/candidates/:candidateId` = `candidate`；
 * `/settings` 的子页是 `settings-users` 等），因此按「`route.name === 导航项 name`」判断
 * 会在候选人详情、捡漏详情、房间内、设置子页全部丢失高亮。
 * `RouterLink` 自带的 `router-link-active` 同样失效——它要求目标的 route record 出现在
 * 当前路由的 `matched` 链里，而兄弟路由不在其中。
 *
 * 所以按**路径归属**判定：与目标路径相同，或位于其 `/` 子层级下，即算命中。
 */
export function isNavPathActive(currentPath: string, targetPath: string): boolean {
  // 根路径是一切路径的前缀，必须精确相等。
  if (targetPath === '/') return currentPath === '/'
  return currentPath === targetPath || currentPath.startsWith(`${targetPath}/`)
}
