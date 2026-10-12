import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import Toast from "./Toast";

describe("Toast", () => {
  it("announces the message and dismisses on click", async () => {
    const onDismiss = vi.fn();
    const user = userEvent.setup();
    render(<Toast message="Requeued abc" onDismiss={onDismiss} durationMs={100000} />);

    expect(screen.getByRole("status")).toHaveTextContent("Requeued abc");
    await user.click(screen.getByRole("button", { name: "Dismiss notification" }));
    expect(onDismiss).toHaveBeenCalledOnce();
  });

  it("auto-dismisses after the duration", () => {
    vi.useFakeTimers();
    try {
      const onDismiss = vi.fn();
      render(<Toast message="Done" onDismiss={onDismiss} durationMs={1000} />);
      expect(onDismiss).not.toHaveBeenCalled();
      vi.advanceTimersByTime(1000);
      expect(onDismiss).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });
});
