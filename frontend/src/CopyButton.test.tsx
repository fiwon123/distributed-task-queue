import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import CopyButton from "./CopyButton";

describe("CopyButton", () => {
  it("writes the value to the clipboard and confirms", async () => {
    // userEvent.setup installs a working navigator.clipboard stub.
    const user = userEvent.setup();

    render(<CopyButton value="abc-123" />);
    await user.click(screen.getByRole("button", { name: "Copy" }));

    await expect(navigator.clipboard.readText()).resolves.toBe("abc-123");
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });
});
