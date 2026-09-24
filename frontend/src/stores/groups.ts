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

  const reorderGroups = (fromIndex: number, toIndex: number) => {
    if (fromIndex < 0 || fromIndex >= groups.value.length) return;
    if (toIndex < 0 || toIndex >= groups.value.length) return;
    const [moved] = groups.value.splice(fromIndex, 1);
    if (!moved) return;
    groups.value.splice(toIndex, 0, moved);
  };

  // 按拖拽后的完整展示顺序（自己的分组 + 只读共享分组的混排列表）拆分写回：
  //   - 自己的分组：按新顺序整体替换 groups 数组（保留原对象引用）；
  //   - 完整 id 顺序（含共享分组）：写入 groupOrder 作为本用户的持久化偏好。
  const applyMergedGroupOrder = (list: NavGroup[]) => {
    const sharedIds = new Set((sharedGroups.value || []).map((g) => g.id));
    const ownIds = new Set(groups.value.map((g) => g.id));
    groups.value = list.filter((g) => !sharedIds.has(g.id) && ownIds.has(g.id));
    groupOrder.value = list.map((g) => g.id);
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
