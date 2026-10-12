import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import Skeleton from "./Skeleton";

describe("Skeleton", () => {
  it("renders the requested number of decorative bars", () => {
    const { container } = render(<Skeleton lines={3} />);

    const root = container.querySelector(".skeleton");
    expect(root).toHaveAttribute("aria-hidden", "true");
    expect(root?.querySelectorAll(".skeleton-line")).toHaveLength(3);
  });

  it("defaults to four lines", () => {
    const { container } = render(<Skeleton />);
    expect(container.querySelectorAll(".skeleton-line")).toHaveLength(4);
  });
});
