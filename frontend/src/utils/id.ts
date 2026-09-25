// 轻量唯一 id 生成：时间戳 + 随机后缀，避免同一毫秒内连续创建（如批量添加书签、
// 快速新建组件）时 Date.now() 撞车产生重复 id。
export const genId = (prefix = ""): string =>
  `${prefix}${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`;
