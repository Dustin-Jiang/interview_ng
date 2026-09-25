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
  菜单含「复制消息」「引用」+ 本人 2 分钟内的「编辑」「撤回」。**正在编辑这条时触发区整块禁用**：
  编辑框要的是系统的复制/粘贴/选中菜单，所以此时既不 preventDefault 也不拦长按（reka 的
  `disabled` 语义，见 `ContextMenuTrigger`），右键/长按回到浏览器原生菜单；取消编辑走 Esc
  或气泡下方的取消按钮。
  动作成功后只发 `changed`：列表内容由持有方（权威拉取）决定，本组件不持状态。
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'

import { candidateApi } from '@/api/http'
import { useAuth } from '@/composables/useAuth'
import { useConfirmAction } from '@/composables/useConfirmAction'
import {
  canModifyMessage,
  continuesGroup,
  senderLabel,
  senderDisplayName,
  senderDepartmentLabel,
  summarizeReactions,
  type ReactionSummary,
} from '@/domain/messages'
import { copyText } from '@/lib/clipboard'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'
import { PERMISSIONS, type Message } from '@/models'

import { Badge } from '@/components/ui/badge'
import { ContextMenu, ContextMenuTrigger } from '@/components/ui/context-menu'
import { chatBubbleVariants, quotedBlockVariants } from '@/components/ui/tokens'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import MessageContextMenu from '@/components/app/MessageContextMenu.vue'
import MessageEditor from '@/components/app/MessageEditor.vue'
import MessageReactions from '@/components/app/MessageReactions.vue'
import QuotedMessageRef from '@/components/app/QuotedMessageRef.vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  /** 按时间升序的消息列表。 */
  messages: Message[]
}>()

/** 编辑/撤回成功：内容已落库，持有方据此重拉或就地更新；`quote` = 请求引用某条记录（持有方接管待发引用）。 */
const emit = defineEmits<{ changed: []; quote: [message: Message] }>()

const { currentUserId, hasPermission } = useAuth()

/**
 * 能否写这条记录（`rooms.chat`）：表情回复、引用、编辑**同属「可写」这一枚权限**，
 * 故统一用 `canChat` 表达。表情与引用不设时间窗口（任何档位都可），编辑另受
 * `canModifyMessage` 的两分钟窗口约束。
 */
const canChat = computed(() => hasPermission(PERMISSIONS.ROOMS_CHAT))

function reactable(m: Message): boolean {
  return canChat.value && m.candidate_id > 0
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
  return senderLabel(m.sender_id, currentUserId.value, senderDisplayName(m))
}

function isOwn(m: Message): boolean {
  return m.sender_id != null && m.sender_id === currentUserId.value
}

/** id → 消息（本页已加载的记录）：引用块据此现查被引用消息——服务端不存内容快照。 */
const byId = computed(() => new Map(props.messages.map((m) => [m.id, m])))

/** 取被引用消息；查不到（已撤回 = 物理删除，或不在当前记录窗口内）返回 undefined，由 QuotedMessageRef 显示占位。 */
const quotedOf = (id: number) => byId.value.get(id)

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

          <!-- 引用块：**在气泡外、气泡正上方**（贴同一侧、与气泡同一收缩宽度上限）——
               引用是「这条在回哪条」的说明，不该挤进气泡、也不该随着气泡的两种底色变样。
               被引用消息从本页已加载记录里按 id 现查（服务端不存内容快照，查不到即「已撤回」）。
               表面与输入区「正在引用」条共用 `quotedBlockVariants`（同一视觉语言）。 -->
          <div
            v-if="m.reply_to_id"
            :class="cn(quotedBlockVariants(), 'max-w-[85%] sm:max-w-[75%]')"
          >
            <QuotedMessageRef :message="quotedOf(m.reply_to_id)" />
          </div>

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

      <!-- 弹层内容拆到 `MessageContextMenu`（本组件接近 400 行上限）：行为与无障碍属性逐字保留，
           数据与写入动作仍由本组件持有，经 props/emits 往返。 -->
      <MessageContextMenu
        :reactable="reactable(m)"
        :modifiable="modifiable(m)"
        :can-chat="canChat"
        :mine="mineEmojis(m)"
        :details="reactionDetails.get(m.id) ?? []"
        @react="toggleReaction(m, $event)"
        @copy="copyMessage(m)"
        @quote="emit('quote', m)"
        @edit="startEdit(m)"
        @recall="requestRecall(m)"
      />
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
