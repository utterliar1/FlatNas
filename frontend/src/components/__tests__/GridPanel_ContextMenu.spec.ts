// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount, VueWrapper } from '@vue/test-utils';
import GridPanel from '../GridPanel.vue';
import { createTestingPinia } from '@pinia/testing';

// Mock dependencies
vi.mock('vue-draggable-plus', () => ({
  VueDraggable: {
    template: '<div><slot /></div>',
    props: ['modelValue', 'group', 'disabled', 'sort', 'handle', 'move', 'animation', 'forceFallback', 'ghostClass']
  }
}));

vi.mock('grid-layout-plus', () => ({
  GridLayout: {
    template: '<div><slot /></div>',
    props: ['layout', 'col-num', 'row-height', 'is-draggable', 'is-resizable', 'vertical-compact', 'use-css-transforms', 'margin']
  },
  GridItem: {
    template: '<div class="grid-item"><slot /></div>',
    props: ['x', 'y', 'w', 'h', 'i', 'drag-allow-from', 'drag-ignore-from']
  }
}));

// Mock composables
vi.mock('../composables/useWallpaperRotation', () => ({ useWallpaperRotation: () => { } }));

// OverlayMotion 默认 teleportTo="body"，内容会被传送到 document.body，
// 导致 wrapper.find() 查不到浮层里的元素。这里替换成内联渲染（保留 show 门控），
// 使上下文菜单与删除确认弹窗仍留在组件树内，便于断言。
vi.mock('@/components/base/OverlayMotion.vue', () => ({
  default: {
    name: 'OverlayMotion',
    props: {
      show: { type: Boolean, default: false },
      zIndex: { default: 50 },
      closeOnOverlay: { type: Boolean, default: false },
      teleportTo: { default: 'body' },
      teleportDisabled: { type: Boolean, default: false },
      overlayClass: { default: '' },
      panelClass: { default: '' },
      panelStyle: { default: undefined },
      variant: { default: 'dialog' },
      appear: { type: Boolean, default: true },
      panelTag: { default: 'div' }
    },
    template: '<div v-if="show" data-overlay-motion><slot /></div>'
  }
}));
vi.mock('../composables/useDevice', () => ({
  useDevice: () => ({ deviceKey: { value: 'desktop' }, isMobile: { value: false } })
}));

// Mock utils
vi.mock('../utils/gridLayout', () => ({
  generateLayout: (widgets: Record<string, unknown>[]) => widgets.map((w: Record<string, unknown>) => ({ ...w, i: w.id, x: 0, y: 0, w: 1, h: 1 })),
  compactVertical: (layout: unknown[]) => layout
}));
vi.mock('@/utils/network', () => ({
  isInternalNetwork: () => false,
  getNetworkConfig: () => ({}),
  // GridPanel 会调用该函数决定组件走内网/外网地址，mock 必须提供同名导出
  computeEffectiveNetworkMode: () => ({ isLan: false, reason: 'mock', measuredLatencyMs: 0 })
}));

describe('GridPanel Context Menu', () => {
  let wrapper: VueWrapper;

  beforeEach(() => {
    vi.clearAllMocks();

    wrapper = mount(GridPanel, {
      global: {
        plugins: [
          createTestingPinia({
            createSpy: () => vi.fn().mockResolvedValue(undefined),
            // 注意：main store 的 widgets / groups / isLogged 都是 computed 转发
            // （computed(() => widgetsStore.widgets) 等），initialState 无法覆盖 computed。
            // 必须直接 seed 真正的来源 store：widgets / groups / auth。
            initialState: {
              auth: {
                token: 'test-token',
                username: 'tester'
              },
              widgets: {
                widgets: [
                  {
                    id: 'div-card-1',
                    type: 'div-card',
                    data: { title: 'Test Div Card' },
                    x: 0, y: 0, w: 1, h: 1, i: 'div-card-1',
                    enable: true,
                    isPublic: true
                  }
                ]
              },
              groups: {
                groups: []
              }
            }
          })
        ],
        stubs: {
          ClockWidget: true,
          SimpleWeatherWidget: true,
          CalendarWidget: true,
          MemoWidget: true,
          TodoWidget: true,
          MusicWidget: true,
          CalculatorWidget: true,
          CountdownWidget: true,
          CountUpWidget: true,
          IframeWidget: true,
          BookmarkWidget: true,
          HotWidget: true,
          ClockWeatherWidget: true,
          AmapWeatherWidget: true,
          RssWidget: true,
          CustomCssWidget: true,
          FileTransferWidget: true,
          IconShape: true,
          MiniPlayer: true,
          AppSidebar: true,
          EditModal: true,
          SettingsModal: true,
          GroupSettingsModal: true,
          LoginModal: true,
          SizeSelector: true,
          transition: false
        }
      }
    });
    // store = useMainStore();
  });

  it('renders div-card widget correctly', () => {
    const divCard = wrapper.find('.div-card-click-target');
    expect(divCard.exists()).toBe(true);
    expect(divCard.text()).toContain('Test Div Card');
  });

  it('opens context menu on right click on div-card', async () => {
    const divCard = wrapper.find('.div-card-click-target');
    await divCard.trigger('contextmenu.prevent');

    const menu = wrapper.find('[data-grid-context-menu]');
    expect(menu.exists()).toBe(true);
    expect(menu.isVisible()).toBe(true);

    // Check menu items
    expect(menu.text()).toContain('编辑卡片');
    expect(menu.text()).toContain('删除卡片');

    // Check SVGs are present (w-4 h-4 class)
    const svgs = menu.findAll('svg.w-4.h-4');
    expect(svgs.length).toBeGreaterThan(0);
  });

  it('clicking delete calls confirm delete logic', async () => {
    const divCard = wrapper.find('.div-card-click-target');
    await divCard.trigger('contextmenu.prevent');

    const menu = wrapper.find('[data-grid-context-menu]');
    // Find delete button (last item usually)
    const items = menu.findAll('[role="menuitem"]');
    const deleteBtn = items[items.length - 1];

    if (!deleteBtn) throw new Error('Delete button not found');
    expect(deleteBtn.text()).toContain('删除卡片');
    await deleteBtn.trigger('click');

    // Check if delete confirm modal is shown
    expect(wrapper.text()).toContain('删除确认');
  });
});
