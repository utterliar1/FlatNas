// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";
import { mount } from "@vue/test-utils";
import { createAppI18n } from "@/plugins/i18n";
import MiniPlayer from "./MiniPlayer.vue";

const { mockConfig } = vi.hoisted(() => ({
  mockConfig: { autoPlayMusic: false },
}));

vi.mock("../stores/main", () => ({
  useMainStore: vi.fn(() => ({
    isLogged: true,
    appConfig: mockConfig,
    activeMusicPlayer: "mini-player",
    getAssetUrl: (u: string) => u,
    token: "t",
  })),
}));

vi.mock("@vueuse/core", () => ({
  useStorage: (_key: string, initial: unknown) => ref(initial),
}));

describe("MiniPlayer lifecycle", () => {
  afterEach(() => {
    mockConfig.autoPlayMusic = false;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("clears pending timers on unmount", () => {
    vi.useFakeTimers();
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");

    const wrapper = mount(MiniPlayer, {
      global: { plugins: [createAppI18n()] },
    });

    wrapper.unmount();

    expect(clearTimeoutSpy).toHaveBeenCalled();
  });

  it("detaches window gesture listeners on unmount when autoplay is pending", () => {
    mockConfig.autoPlayMusic = true;
    const removeSpy = vi.spyOn(window, "removeEventListener");

    const wrapper = mount(MiniPlayer, {
      global: { plugins: [createAppI18n()] },
    });

    wrapper.unmount();

    expect(removeSpy).toHaveBeenCalled();
  });
});
