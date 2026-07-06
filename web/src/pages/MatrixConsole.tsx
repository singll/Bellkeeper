import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import { useToast } from '@/components/Toast'
import { formatDateShort } from '@/utils/format'
import type { MatrixRoom } from '@/types'

// MatrixConsole 是 1.0 Matrix 7→3 页重构的合并页（§2.3.3 T9）：
// 合并原 Rooms/Commands/Notifications/Events/CommandLogs 为单页 tab 视图。
// 只读列表 + 极少写（仅改 room_type）；创建/删除房间等管理操作移除，走 Admin API/Matrix 命令。
const MatrixConsole: Component = () => {
  const toast = useToast()
  const [tab, setTab] = createSignal<'rooms' | 'commands' | 'notifications' | 'events' | 'logs'>('rooms')

  const [rooms, { refetch: refetchRooms }] = createResource(() => matrixApi.listRooms({ page: 1, page_size: 100 }))
  const [commands] = createResource(() => matrixApi.listCommands())
  const [notifications] = createResource(() => matrixApi.listNotifications({ page: 1, page_size: 50 }))
  const [events] = createResource(() => matrixApi.listEvents({ page: 1, page_size: 50 }))
  const [logs] = createResource(() => matrixApi.listCommandLogs({ page: 1, per_page: 50 }))

  const handleRoomTypeChange = async (room: MatrixRoom, newType: string) => {
    try {
      await matrixApi.updateRoom(room.id, { room_type: newType })
      toast.success('房间类型已更新')
      refetchRooms()
    } catch (err) {
      toast.error('更新失败: ' + (err as Error).message)
    }
  }

  const tabs = [
    { key: 'rooms', label: '房间' },
    { key: 'commands', label: '命令' },
    { key: 'notifications', label: '通知' },
    { key: 'events', label: '事件' },
    { key: 'logs', label: '命令日志' },
  ] as const

  return (
    <div class="animate-fade-in">
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">Matrix 控制台</h1>
        <p class="text-sm text-dark-400 mt-1">房间 / 命令 / 通知 / 事件（只读，仅 room_type 可改）</p>
      </div>

      {/* Tabs */}
      <div class="flex gap-1 mb-6 border-b border-dark-700 overflow-x-auto">
        <For each={tabs}>
          {(t) => (
            <button
              class={`px-4 py-2 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                tab() === t.key
                  ? 'border-violet-500 text-white'
                  : 'border-transparent text-dark-400 hover:text-white'
              }`}
              onClick={() => setTab(t.key)}
            >
              {t.label}
            </button>
          )}
        </For>
      </div>

      <Show when={tab() === 'rooms'}>
        <div class="card overflow-hidden">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">房间名</th>
                <th class="th">Room ID</th>
                <th class="th">类型</th>
                <th class="th">活跃</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={rooms()?.data ?? []}>
                {(room) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td font-medium text-white">{room.room_name || '-'}</td>
                    <td class="td text-dark-400 font-mono text-xs">{room.room_id}</td>
                    <td class="td">
                      <select
                        class="input-sm"
                        value={room.room_type}
                        onChange={(e) => handleRoomTypeChange(room, e.currentTarget.value)}
                      >
                        <option value="notification">notification</option>
                        <option value="command">command</option>
                        <option value="alert">alert</option>
                        <option value="qa">qa</option>
                      </select>
                    </td>
                    <td class="td">
                      <span class={`badge ${room.is_active ? 'badge-green' : 'badge-gray'}`}>
                        {room.is_active ? '活跃' : '停用'}
                      </span>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={tab() === 'commands'}>
        <div class="card overflow-hidden">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">命令</th>
                <th class="th">描述</th>
                <th class="th">权限</th>
                <th class="th">启用</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={commands()?.data ?? []}>
                {(cmd) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td font-mono text-violet-300">{cmd.command}</td>
                    <td class="td text-dark-300">{cmd.description || '-'}</td>
                    <td class="td"><span class="badge badge-blue">{cmd.permission || 'user'}</span></td>
                    <td class="td">
                      <span class={`badge ${cmd.enabled ? 'badge-green' : 'badge-gray'}`}>
                        {cmd.enabled ? '启用' : '禁用'}
                      </span>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={tab() === 'notifications'}>
        <div class="card overflow-hidden">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">频道</th>
                <th class="th">状态</th>
                <th class="th">重试</th>
                <th class="th">时间</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={notifications()?.data?.items ?? []}>
                {(n) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td">{n.channel_name || n.channel}</td>
                    <td class="td">
                      <span class={`badge ${n.status === 'sent' ? 'badge-green' : n.status === 'failed' ? 'badge-red' : 'badge-gray'}`}>
                        {n.status}
                      </span>
                    </td>
                    <td class="td text-dark-400">{n.retry_count ?? 0}</td>
                    <td class="td text-dark-400 text-xs">{formatDateShort(n.created_at)}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={tab() === 'events'}>
        <div class="card overflow-hidden">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">类型</th>
                <th class="th">房间</th>
                <th class="th">发送者</th>
                <th class="th">时间</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={events()?.data?.items ?? []}>
                {(ev) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td font-mono text-xs text-violet-300">{ev.event_type}</td>
                    <td class="td text-dark-400 font-mono text-xs">{ev.room_id}</td>
                    <td class="td text-dark-400 text-xs">{ev.sender}</td>
                    <td class="td text-dark-400 text-xs">{formatDateShort(ev.created_at)}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={tab() === 'logs'}>
        <div class="card overflow-hidden">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">命令</th>
                <th class="th">状态</th>
                <th class="th">发送者</th>
                <th class="th">时间</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={logs()?.data?.items ?? []}>
                {(log) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td font-mono text-violet-300">{log.command}</td>
                    <td class="td">
                      <span class={`badge ${log.status === 'success' ? 'badge-green' : log.status === 'error' ? 'badge-red' : 'badge-gray'}`}>
                        {log.status}
                      </span>
                    </td>
                    <td class="td text-dark-400 text-xs">{log.sender}</td>
                    <td class="td text-dark-400 text-xs">{formatDateShort(log.created_at)}</td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>
    </div>
  )
}

export default MatrixConsole
