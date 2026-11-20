import { afterEach, expect, it, vi } from "vitest";
import { NavigationEventSource } from "./navigation";

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

it("closes a disabled stream without scheduling a reconnect", () => {
	vi.useFakeTimers();
	let source: FakeEventSource;
	class FakeEventSource extends EventTarget {
		static OPEN = 1;
		static CONNECTING = 0;
		static CLOSED = 2;
		readyState = FakeEventSource.OPEN;
		constructor() {
			super();
			source = this;
		}
		close = vi.fn(() => {
			this.readyState = FakeEventSource.CLOSED;
		});
	}
	vi.stubGlobal("EventSource", FakeEventSource);
	const onDisabled = vi.fn();
	const stream = new NavigationEventSource({ onNavigate: vi.fn(), onDisabled });
	stream.connect();
	source!.dispatchEvent(new Event("disabled"));
	expect(source!.close).toHaveBeenCalledOnce();
	expect(onDisabled).toHaveBeenCalledOnce();
	vi.advanceTimersByTime(70000);
	expect(stream.isConnected()).toBe(false);
});

it("ignores events from a replaced stream", () => {
	vi.useFakeTimers();
	const sources: FakeEventSource[] = [];
	class FakeEventSource extends EventTarget {
		static OPEN = 1;
		static CONNECTING = 0;
		static CLOSED = 2;
		readyState = FakeEventSource.OPEN;
		constructor() {
			super();
			sources.push(this);
		}
		close = vi.fn(() => {
			this.readyState = FakeEventSource.CLOSED;
		});
	}
	vi.stubGlobal("EventSource", FakeEventSource);
	const onDisabled = vi.fn();
	const onNavigate = vi.fn();
	const stream = new NavigationEventSource({ onNavigate, onDisabled });
	stream.connect();
	stream.disconnect();
	stream.connect();
	sources[0].dispatchEvent(new Event("disabled"));
	sources[0].dispatchEvent(new MessageEvent("navigate", { data: '{"url":"/views/inbox"}' }));
	expect(onDisabled).not.toHaveBeenCalled();
	expect(onNavigate).not.toHaveBeenCalled();
	expect(stream.isConnected()).toBe(true);
	stream.disconnect();
});
