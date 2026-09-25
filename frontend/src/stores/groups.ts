import { ref, computed } from "vue";
import { defineStore } from "pinia";
import type { NavItem, NavGroup } from "@/types";

export const useGroupsStore = defineStore("groups", () => {
  const groups = ref<NavGroup[]>([]);
  // 共享分组（多用户共同的书签分组）：后端从管理员数据派生的只读副本。
  // 仅用于渲染复用，绝不参与本用户的保存载荷。
  const sharedGroups = ref<NavGroup[]>([]);
  // 分组混排顺序偏好：记录「自己的分组 + 只读共享分组」在当前用户视图中的
  // 完整排列（仅存分组 id）。只影响本用户的展示顺序，随用户自己的数据文件
  // 持久化，不会影响管理员与其他用户；空数组表示未自定义，按默认顺序展示。
  const groupOrder = ref<string[]>([]);
  const items = computed(() => groups.value.flatMap((g) => g.items));

  const cleanInvalidGroups = () => {
    const seen = new Set<string>();
    groups.value = groups.value.filter((g) => {
      const validId = typeof g.id === "string" && g.id.length > 0;
      const dup = validId && seen.has(g.id);
      if (validId) seen.add(g.id);
      const hasTitle = typeof g.title === "string" && g.title.trim().length > 0;
      const hasItems = Array.isArray(g.items) && g.items.length > 0;
      return validId && (hasTitle || hasItems) && !dup;
    });
  };

  const addGroup = () => {
    try {
      const id = Date.now().toString();
      const index = groups.value.length + 1;
      const title = `新建分组 ${index}`;
      groups.value.push({ id, title, items: [] });
    } catch (e) {
      console.error(e);
    }
  };

  const deleteGroup = (groupId: string, skipConfirm = false) => {
    if (!skipConfirm && !confirm("确定删除？")) return;
    groups.value = groups.value.filter((g) => g.id !== groupId);
  };

  const updateGroupTitle = (groupId: string, newTitle: string) => {
    const group = groups.value.find((g) => g.id === groupId);
    if (group) {
      group.title = newTitle;
    }
  };

  const updateGroup = (groupId: string, updates: Partial<NavGroup>) => {
    const group = groups.value.find((g) => g.id === groupId);
    if (group) {
      Object.assign(group, updates);
    }
  };

  const addItem = (item: NavItem, groupId: string) => {
    const group = groups.value.find((g) => g.id === groupId);
    if (group) {
      group.items.push({ ...item, isPublic: item.isPublic ?? true });
    }
  };

  const updateItem = (updatedItem: NavItem) => {
    for (const group of groups.value) {
      const idx = group.items.findIndex((i) => i.id === updatedItem.id);
      if (idx !== -1) {
        group.items[idx] = updatedItem;
        return;
      }
    }
  };

  const deleteItem = (id: string) => {
    for (const group of groups.value) {
      const idx = group.items.findIndex((i) => i.id === id);
      if (idx !== -1) {
        group.items.splice(idx, 1);
        return;
      }
    }
  };

  // 重建完整混排顺序：以新可见顺序为主干，把偏好中当前不可见的 id
  // （受保护隐藏分组、已失效 id）按原有相对位置插回——
  // 逐个在 oldOrder 中向前找第一个已在新顺序中的 id，插到它后面；
  // 找不到锚点（原本就最靠前）则插到最前面。保证解锁后隐藏分组位置不漂移。
  const rebuildOrderPreservingInvisible = (visibleIds: string[]): string[] => {
    const visibleSet = new Set(visibleIds);
    const oldOrder = groupOrder.value || [];
    const result = [...visibleIds];
    for (const id of oldOrder) {
      if (visibleSet.has(id) || result.includes(id)) continue;
      const idx = oldOrder.indexOf(id);
      let inserted = false;
      for (let i = idx - 1; i >= 0; i--) {
        const ai = result.indexOf(oldOrder[i]);
        if (ai !== -1) {
          result.splice(ai + 1, 0, id);
          inserted = true;
          break;
        }
      }
      if (!inserted) result.unshift(id);
    }
    return result;
  };

  const reorderGroups = (fromIndex: number, toIndex: number) => {
    if (fromIndex < 0 || fromIndex >= groups.value.length) return;
    if (toIndex < 0 || toIndex >= groups.value.length) return;
    const [moved] = groups.value.splice(fromIndex, 1);
    if (!moved) return;
    groups.value.splice(toIndex, 0, moved);
    // 同步维护混排偏好：即使走的是无共享分组的快速路径，也要把新顺序写入
    // groupOrder（并保留锁定状态下不可见的隐藏分组 id 的位置），否则解锁后
    // 隐藏分组的展示位置会随 groups 数组顺序漂移。
    groupOrder.value = rebuildOrderPreservingInvisible(groups.value.map((g) => g.id));
  };

  // 按拖拽后的完整展示顺序（自己的分组 + 只读共享分组的混排列表）拆分写回：
  //   - 自己的分组：按新顺序整体替换 groups 数组（保留原对象引用）；
  //   - 完整 id 顺序（含共享分组）：写入 groupOrder 作为本用户的持久化偏好，
  //     并保留偏好中当前不可见的 id（锁定时的隐藏分组）的原有相对位置。
  const applyMergedGroupOrder = (list: NavGroup[]) => {
    const sharedIds = new Set((sharedGroups.value || []).map((g) => g.id));
    const ownIds = new Set(groups.value.map((g) => g.id));
    groups.value = list.filter((g) => !sharedIds.has(g.id) && ownIds.has(g.id));
    groupOrder.value = rebuildOrderPreservingInvisible(list.map((g) => g.id));
  };

  return {
    groups,
    sharedGroups,
    groupOrder,
    items,
    cleanInvalidGroups,
    addGroup,
    deleteGroup,
    updateGroupTitle,
    updateGroup,
    addItem,
    updateItem,
    deleteItem,
    reorderGroups,
    applyMergedGroupOrder,
  };
});
