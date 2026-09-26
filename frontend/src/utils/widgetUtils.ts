/**
 * Widget-related utility functions extracted from main.ts.
 * These functions handle widget normalization, deduplication, and defaults.
 */

import type { WidgetConfig } from "@/types";
import { genId } from "./id";

/**
 * 保证 widget id 全局唯一。
 *
 * 为什么必须做：网格库 `grid-layout-plus` 在挂载时会校验 `Layout[i].i` 唯一，
 * 一旦重复就 **throw**（`VueGridLayout: Layout[n].i must be unique!`）并中断整棵网格的挂载 ——
 * 表现是首页组件区整个渲染不出来，而不只是一条无害的告警。
 *
 * 传入顺序即优先级：先出现的保留原 id，后出现的冲突项改用派生 id
 * （优先保留 `<type>-` 前缀，便于排查）。
 */
export function ensureUniqueWidgetIds(list: WidgetConfig[]): WidgetConfig[] {
  const used = new Set<string>();
  return list.map((widget) => {
    const id = typeof widget.id === "string" ? widget.id : "";
    if (id && !used.has(id)) {
      used.add(id);
      return widget;
    }
    const prefix = widget.type ? `${widget.type}-` : "";
    let next = genId(prefix);
    while (used.has(next)) next = genId(prefix);
    used.add(next);
    return { ...widget, id: next };
  });
}


/**
 * Create default widget list when no widgets are provided.
 */
export function createDefaultWidgetList(isLoggedIn: boolean): WidgetConfig[] {
  const base: WidgetConfig[] = [
    { id: "w1", type: "clock", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "w2", type: "weather", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "w3", type: "calendar", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "w5", type: "search", enable: true, isPublic: true },
    { id: "w7", type: "quote", enable: true, isPublic: true },
    {
      id: "clockweather",
      type: "clockweather",
      enable: true,
      colSpan: 1,
      rowSpan: 1,
      isPublic: true,
    },
    { id: "sidebar", type: "sidebar", enable: false, isPublic: true },
    {
      id: "file-transfer",
      type: "file-transfer",
      enable: true,
      colSpan: 2,
      rowSpan: 2,
      isPublic: true,
    },
    { id: "memo", type: "memo", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "todo", type: "todo", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    {
      id: "calculator",
      type: "calculator",
      enable: true,
      colSpan: 1,
      rowSpan: 1,
      isPublic: true,
    },
    { id: "ip", type: "ip", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "hot", type: "hot", enable: true, colSpan: 1, rowSpan: 1, isPublic: true },
    { id: "player", type: "player", enable: true, colSpan: 2, rowSpan: 1, isPublic: true },
    {
      id: "status-monitor",
      type: "status-monitor",
      enable: false,
      colSpan: 1,
      rowSpan: 1,
      isPublic: true,
    },
  ];

  // Filter out login-only widgets for guests
  if (!isLoggedIn) {
    return base.filter((w) => {
      const loginOnly = ["file-transfer", "sidebar", "status-monitor"];
      return !loginOnly.includes(w.id);
    });
  }

  return base;
}

/**
 * Normalize incoming widget list: fix duplicates, missing defaults, ID conflicts.
 */
export function normalizeIncomingWidgets(
  input?: WidgetConfig[],
  isLoggedIn?: boolean,
): WidgetConfig[] {
  const nextWidgets = Array.isArray(input) ? input.map((widget) => ({ ...widget })) : [];

  if (nextWidgets.length === 0) {
    return createDefaultWidgetList(!!isLoggedIn);
  }

  // Fix memo type
  const memoW = nextWidgets.find((widget) => widget.id === "memo");
  if (memoW && memoW.type !== "memo") {
    memoW.type = "memo";
  }

  // 已剥离的功能组件：Docker 管理、宿主机状态。
  // 宿主机状态读取的是容器所在宿主（爱快路由器）的 CPU/内存/磁盘指标，本机部署场景下无意义。
  // 这里过滤掉历史数据中残留的对应组件，避免渲染成"未知组件"占位卡片。
  const REMOVED_WIDGET_TYPES = ["docker", "system-status"];
  const listFilteredByRemoved = nextWidgets.filter(
    (widget) =>
      !REMOVED_WIDGET_TYPES.includes(widget.id) && !REMOVED_WIDGET_TYPES.includes(widget.type),
  );

  // Normalize File Transfer widget (deduplicate)
  const fileTransferList = listFilteredByRemoved.filter((widget) => widget.type === "file-transfer");
  if (fileTransferList.length > 1) {
    const keep =
      fileTransferList.find((widget) => widget.id === "file-transfer") || fileTransferList[0]!;
    const filtered = listFilteredByRemoved.filter(
      (widget) => widget.type !== "file-transfer" || widget === keep,
    );
    if (
      keep.id !== "file-transfer" &&
      !filtered.some((widget) => widget.id === "file-transfer" && widget.type !== "file-transfer")
    ) {
      keep.id = "file-transfer";
    }
    nextWidgets.length = 0;
    nextWidgets.push(...filtered);
  } else if (
    fileTransferList.length === 1 &&
    fileTransferList[0]!.id !== "file-transfer" &&
    !listFilteredByRemoved.some(
      (widget) => widget.id === "file-transfer" && widget.type !== "file-transfer",
    )
  ) {
    fileTransferList[0]!.id = "file-transfer";
    nextWidgets.length = 0;
    nextWidgets.push(...listFilteredByRemoved);
  } else if (fileTransferList.length === 0 && isLoggedIn) {
    listFilteredByRemoved.push({
      id: "file-transfer",
      type: "file-transfer",
      enable: true,
      colSpan: 2,
      rowSpan: 2,
      isPublic: true,
    });
    nextWidgets.length = 0;
    nextWidgets.push(...listFilteredByRemoved);
  } else {
    nextWidgets.length = 0;
    nextWidgets.push(...listFilteredByRemoved);
  }

  // Keep normalization and "restore defaults" aligned to the same source of truth.
  // In guest mode, only use defaults as fillers for missing types that are already marked isPublic in user data.
  // In logged-in mode, freely add missing defaults.
  const isGuest = !isLoggedIn;
  for (const fallback of createDefaultWidgetList(!!isLoggedIn)) {
    if (!nextWidgets.some((widget) => widget.type === fallback.type)) {
      if (isGuest) {
        continue;
      }
      nextWidgets.push(fallback);
    }
  }

  // 补齐 fallback 后必须再保证 id 唯一：
  // 用户数据里可能存在「同一个 id 被别的 type 占用」的历史残留
  // （例如 id 为 `w1` 的组件其实是 music，而默认列表里 `w1` 是 clock）。
  // 若只按 type 判重就直接 push fallback，会得到一个重复 id 的组件，
  // 进而让 grid-layout-plus 在挂载时抛 "Layout[i].i must be unique!" 并中断渲染。
  return ensureUniqueWidgetIds(nextWidgets);
}
