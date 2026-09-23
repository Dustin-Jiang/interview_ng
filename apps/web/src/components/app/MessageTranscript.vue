<!--
  MessageTranscript —— 消息列表（只读回放 + 本人消息的编辑/撤回）。
  房间实时聊天与房间外的归档场景（候选人查看页）共用一份渲染，保证同一视觉语言
  （消息头同样是「姓名 + 部门头衔 + 时间」），也保证编辑/撤回只有一处实现。

  结构参照 shadcn Message/Bubble：**名头（姓名 + 部门 + 时间）→ 气泡 → 表情胶囊**；
  同一发送者 3 分钟内的连续消息归为一组（`domain/messages.ts#continuesGroup`）：组内只首条出名头、
  气泡上圆角相连、间距收紧，其余只留时间。表情胶囊排在气泡**下方的一行**（`MessageReactions`，不重叠气泡）。
  **本组件不自带 live region**：`role=log` 由外层滚动容器（房间页）持有，避免嵌套两层播报区。

  交互：在气泡上**右键**（触屏长按、键盘菜单键同效）打开菜单——「自己发送且两分钟内」的记录
  才能编辑/撤回（`domain/messages.ts#canModifyMessage`，窗口口径与后端 `state.MessageModifyWindow`
  一致）；其余记录触发区禁用，右键即浏览器默认菜单。编辑就地改、撤回走二次确认（不可逆）。
  菜单含「复制消息」+ 本人 2 分钟内的「编辑」「撤回」。**正在编辑这条时触发区整块禁用**：
  编辑框要的是系统的复制/粘贴/选中菜单，所以此时既不 preventDefault 也不拦长按（reka 的
  `disabled` 语义，见 `ContextMenuTrigger`），右键/长按回到浏览器原生菜单；取消编辑走 Esc
  或气泡下方的取消按钮。
  动作成功后只发 `changed`：列表内容由持有方（权威拉取）决定，本组件不持状态。
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Copy, Pencil, Undo2 } from 'lucide-vue-next'
import { toast } from 'vue-sonner'

import { candidateApi } from '@/api/http'
import { useAuth } from '@/composables/useAuth'
import { useConfirmAction } from '@/composables/useConfirmAction'
import {
  REACTION_EMOJIS,
  canModifyMessage,
  continuesGroup,
  senderLabel,
  senderDepartmentLabel,
  summarizeReactions,
  type ReactionSummary,
} from '@/domain/messages'
import { copyText } from '@/lib/clipboard'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'
import { PERMISSIONS, type Message } from '@/models'

import { Badge } from '@/components/ui/badge'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import { chatBubbleVariants } from '@/components/ui/tokens'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import MessageEditor from '@/components/app/MessageEditor.vue'
import MessageReactions from '@/components/app/MessageReactions.vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  /** 按时间升序的消息列表。 */
  messages: Message[]
}>()

/** 编辑/撤回成功：内容已落库，持有方据此重拉或就地更新。 */
const emit = defineEmits<{ changed: [] }>()

const { currentUserId, hasPermission } = useAuth()

/** 能否回复表情：与写入记录同一枚权限（房间聊天）；不设时间窗口，任何档位都可回。 */
const canReact = computed(() => hasPermission(PERMISSIONS.ROOMS_CHAT))

function reactable(m: Message): boolean {
  return canReact.value && m.candidate_id > 0
}

/**
 * 窗口判定的「现在」：15 秒走一格，超时后入口自动收起（服务端另有权威判定，前端只控制显示）。
 */
const now = ref(Date.now())
let ticker: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  ticker = setInterval(() => (now.value = Date.now()), 15_000)
})
onBeforeUnmount(() => {
  if (ticker) clearInterval(ticker)
})

function labelOf(m: Message): string {
  return senderLabel(m.sender_id, currentUserId.value, m.sender?.name || m.sender?.username)
}

function isOwn(m: Message): boolean {
  return m.sender_id != null && m.sender_id === currentUserId.value
}

function modifiable(m: Message): boolean {
  return canModifyMessage(m, currentUserId.value, now.value)
}

// ---- 表情回复 ----
/** 每条消息的表情汇总（一次性算好，避免每次渲染重复聚合）。 */
const summaries = computed(() => {
  const map = new Map<number, ReactionSummary[]>()
  for (const m of props.messages) {
    const list = summarizeReactions(m.reactions, currentUserId.value)
    if (list.length > 0) map.set(m.id, list)
  }
  return map
})

/**
 * 逐条算「组内角色」：`continues` = 上方还有同组气泡（不出名头、圆角相连、间距收紧），
 * `hasReactions` = 下方挂了表情胶囊（要留出胶囊占位）。
 */
const layout = computed(() =>
  props.messages.map((m, i) => ({
    continues: continuesGroup(props.messages[i - 1], m),
    // 下一条若仍属同组，本条的胶囊会和下一条的名头/气泡靠得很近，间距另算。
    reactions: (summaries.value.get(m.id)?.length ?? 0) > 0,
  })),
)

function reactionLabel(m: Message): string {
  const list = summaries.value.get(m.id) ?? []
  return `表情回复：${list.map((r) => `${r.emoji} ${r.count}`).join('，')}`
}

/**
 * 右键菜单里的「谁回了什么」明细：每个表情一行，按回复先后列出回复人
 * （展示名复用 `senderLabel`：自己 → 「我」，已删用户 → 「已删除用户」），无部门则不附加头衔。
 */
const reactionDetails = computed(() => {
  const map = new Map<number, { emoji: string; count: number; names: string }[]>()
  for (const m of props.messages) {
    const list = summaries.value.get(m.id)
    if (!list || list.length === 0) continue
    map.set(
      m.id,
      list.map((r) => ({
        emoji: r.emoji,
        count: r.count,
        names: r.reactors
          .map((a) => {
            const label = senderLabel(a.user_id, currentUserId.value, a.name)
            return a.department ? `${label}·${a.department}` : label
          })
          .join('、'),
      })),
    )
  }
  return map
})

/** 某条消息有没有可展示的表情明细。 */
function hasReactions(m: Message): boolean {
  return (summaries.value.get(m.id)?.length ?? 0) > 0
}

/** 我在某条消息上已经回过的表情集合（菜单网格里标出，避免误点成撤回）。 */
function mineEmojis(m: Message): Set<string> {
  return new Set((summaries.value.get(m.id) ?? []).filter((r) => r.mine).map((r) => r.emoji))
}

/** 表情写入中：防连点（同一时刻至多一个请求）。 */
const reacting = ref(false)

/** 点一个表情 = 开关我自己的这个表情（幂等；服务端权威，成功后就地重拉）。 */
async function toggleReaction(m: Message, emoji: string): Promise<void> {
  if (!reactable(m) || reacting.value) return
  const mine = (m.reactions ?? []).some((r) => r.user_id === currentUserId.value && r.emoji === emoji)
  reacting.value = true
  try {
    await candidateApi.setReaction(m.candidate_id, m.id, emoji, !mine)
    emit('changed')
  } catch (e) {
    toastError(e)
  } finally {
    reacting.value = false
  }
}

// ---- 就地编辑 ----
/**
 * 复制这条记录的内容到剪贴板。
 * 走 `lib/clipboard` 而不是裸 `navigator.clipboard`：后者在 http 部署（`http://<内网IP>:8080`）
 * 是 undefined，会静默失效；失败就明确提示手动选择文字，不假装成功。
 */
async function copyMessage(m: Message): Promise<void> {
  const ok = await copyText(m.content)
  if (ok) toast.success('已复制这条记录')
  else toastError(null, '复制失败，请手动选择文字复制')
}

const editingId = ref<number | null>(null)
const editDraft = ref('')
const saving = ref(false)

/** 该条是否正在就地编辑（编辑态下气泡换成输入框，胶囊一并收起）。 */
function isEditing(m: Message): boolean {
  return editingId.value === m.id
}

/** 进入编辑态：草稿取自当前正文，聚焦由 `MessageEditor` 自己负责（open 变 true 即聚焦）。 */
function startEdit(m: Message): void {
  editingId.value = m.id
  editDraft.value = m.content
}

function cancelEdit(): void {
  editingId.value = null
  editDraft.value = ''
}

async function saveEdit(m: Message): Promise<void> {
  const text = editDraft.value.trim()
  if (!text || saving.value) return
  if (text === m.content) {
    cancelEdit()
    return
  }
  saving.value = true
  try {
    await candidateApi.updateMessage(m.candidate_id, m.id, text)
    cancelEdit()
    emit('changed')
  } catch (e) {
    toastError(e)
  } finally {
    saving.value = false
  }
}

// ---- 撤回（不可逆，走二次确认） ----
const {
  target: recallTarget,
  loading: recallLoading,
  request: requestRecall,
  onOpenChange: onRecallOpenChange,
  confirm: confirmRecall,
} = useConfirmAction<Message>({
  action: async (m) => {
    await candidateApi.removeMessage(m.candidate_id, m.id)
    emit('changed')
  },
  success: () => '已撤回这条记录',
})
</script>

<template>
  <div class="flex w-full min-w-0 flex-col">
    <ContextMenu v-for="(m, index) in messages" :key="m.id">
      <!-- 触发区即整条消息：可改（本人 2 分钟内）、可回表情、或已有表情明细（谁回了什么）时
           右键/长按/菜单键出菜单，其余禁用（保留浏览器默认菜单）。 -->
      <ContextMenuTrigger
        :disabled="isEditing(m) || (!modifiable(m) && !reactable(m) && !hasReactions(m))"
        as-child
      >
        <div
          class="flex min-w-0 max-w-full flex-col gap-1 rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          :class="[
            isOwn(m) ? 'items-end' : 'items-start',
            // 组内间距收紧（4px），组间 12px；行内（名头 / 气泡 / 胶囊）固定 4px。
            layout[index].continues ? 'mt-1' : index > 0 ? 'mt-3' : '',
          ]"
          :tabindex="modifiable(m) || reactable(m) || hasReactions(m) ? 0 : undefined"
        >
          <!-- 名头：姓名 + 部门头衔 + 时间——**整块**只在「新的一组」出现。
               分组规则：与上一条同一发送者、且间隔 ≤3 分钟（`domain/messages.ts#continuesGroup`）
               → 并入上一组，名头与时间一起隐藏（组内只留气泡，间距 4px）；否则重新出名头 + 时间。
               隐藏的时间仍以 sr-only 留在无障碍树里（面试档案按条有据可查），不占视觉空间。 -->
          <div
            v-if="!layout[index].continues"
            class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 px-1 text-xs text-muted-foreground"
          >
            <span class="font-medium">{{ labelOf(m) }}</span>
            <Badge v-if="senderDepartmentLabel(m)" variant="outline" class="px-1.5 py-0 font-normal">
              {{ senderDepartmentLabel(m) }}
            </Badge>
            <time>{{ formatDateTime(m.created_at) }}</time>
          </div>
          <time v-else class="sr-only">{{ formatDateTime(m.created_at) }}</time>

          <!-- 编辑态：气泡换成多行输入（Enter 保存 / Shift+Enter 换行 / Esc 取消） -->
          <MessageEditor
            v-if="editingId === m.id"
            v-model="editDraft"
            :open="editingId === m.id"
            :saving="saving"
            @save="saveEdit(m)"
            @cancel="cancelEdit"
          />

          <div
            v-else
            :class="cn(chatBubbleVariants({ side: isOwn(m) ? 'own' : 'other', grouped: layout[index].continues }))"
          >
            {{ m.content }}
          </div>

          <!-- 表情胶囊：排在气泡下方的一行（不重叠气泡），左右随消息方向对齐。
               千万别为了「贴合气泡边缘」改成绝对定位并绑定气泡宽度——气泡盒子是收缩宽度，
               短消息（如「好」）会把胶囊挤成几十像素宽、折成一列并倒压到上方名头上
               （实测：气泡 31.5px → 胶囊盒 34px 宽 / 82px 高，top 越过名头）。 -->
          <MessageReactions
            v-if="!isEditing(m) && layout[index].reactions"
            class="max-w-full"
            :summary="summaries.get(m.id) ?? []"
            :reactable="reactable(m)"
            :busy="reacting"
            @toggle="toggleReaction(m, $event)"
          />
        </div>
      </ContextMenuTrigger>

      <!-- 弹层默认 min-w 只有 9rem：6 列表情网格需要更宽的底板，顺带让「谁回了什么」一行放得下。
           触屏再放宽到 17rem——手机窄屏下 13rem 会把每格表情压成 30px 宽的细条（高度却是 44px），
           「过细」得不像可点的目标；17rem 让 6 列各约 42px ≈ 方格。上限 `100vw-1rem` 防溢出。 -->
      <ContextMenuContent
        class="max-h-[var(--reka-context-menu-content-available-height)] min-w-[13rem] max-w-[calc(100vw-1rem)] overflow-y-auto max-lg:min-w-[17rem]"
      >
        <!-- 表情回复：24 个表情铺成 6×4 网格（与 `REACTION_EMOJIS` 的分组顺序一致，一点即回，
             不必再开子菜单）。
             已经回过的表情用实底标出（MenuItem 的 aria-checked + data-state，见下），避免「再点一下把它撤了」的意外。 -->
        <div v-if="reactable(m)" class="grid grid-cols-6 gap-0.5 p-1" role="presentation">
          <ContextMenuItem
            v-for="e in REACTION_EMOJIS"
            :key="e"
            class="justify-center px-0 text-base leading-none"
            :class="mineEmojis(m).has(e) ? 'bg-primary text-primary-foreground' : ''"
            role="menuitemcheckbox"
            :aria-checked="mineEmojis(m).has(e)"
            :aria-label="mineEmojis(m).has(e) ? `撤回 ${e}` : `用 ${e} 回复`"
            @select="toggleReaction(m, e)"
          >
            <span aria-hidden="true">{{ e }}</span>
          </ContextMenuItem>
        </div>
        <ContextMenuSeparator v-if="reactable(m) && (hasReactions(m) || modifiable(m))" />

        <!-- 谁回了什么：每个表情一行，列出回复人（姓名·部门，自己显示「我」）。
             纯展示区，不是菜单项——避免方向键停在无动作的行上。 -->
        <div v-if="hasReactions(m)" class="space-y-0.5 px-2 py-1.5" role="group" aria-label="表情回复明细">
          <div
            v-for="d in reactionDetails.get(m.id)"
            :key="d.emoji"
            class="flex items-baseline gap-2 text-xs"
          >
            <!-- 与右侧姓名同字号、同基线：emoji 用更大的字号或 items-start 会让两者差开 ~2px -->
            <span class="w-4 shrink-0 text-center" aria-hidden="true">{{ d.emoji }}</span>
            <span class="shrink-0 tabular-nums text-muted-foreground">×{{ d.count }}</span>
            <span class="min-w-0 break-words text-foreground/90">{{ d.names }}</span>
          </div>
        </div>
        <ContextMenuSeparator v-if="hasReactions(m) && modifiable(m)" />

        <!-- 复制消息：任何能打开菜单的记录都可复制（含别人的、已过编辑窗口的）。 -->
        <ContextMenuItem @select="copyMessage(m)">
          <Copy class="h-4 w-4" aria-hidden="true" />
          复制消息
        </ContextMenuItem>
        <ContextMenuItem v-if="modifiable(m)" @select="startEdit(m)">
          <Pencil class="h-4 w-4" aria-hidden="true" />
          编辑
        </ContextMenuItem>
        <ContextMenuSeparator v-if="modifiable(m)" />
        <ContextMenuItem v-if="modifiable(m)" destructive @select="requestRecall(m)">
          <Undo2 class="h-4 w-4" aria-hidden="true" />
          撤回
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>

    <!-- 撤回确认（不可逆） -->
    <ConfirmDialog
      :open="!!recallTarget"
      title="撤回这条记录？"
      confirm-text="撤回"
      destructive
      :loading="recallLoading"
      @update:open="onRecallOpenChange"
      @confirm="confirmRecall"
    />
  </div>
</template>
