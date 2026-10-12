import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import QueueComposition from "./QueueComposition";

describe("QueueComposition", () => {
  it("renders a labelled bar and a legend for the present statuses", () => {
    const { container } = render(
      <QueueComposition counts={{ pending: 2, running: 0, completed: 3, failed: 0, dead: 1 }} />,
    );

    const bar = screen.getByRole("img", {
      name: "Queue composition: 2 pending, 3 completed, 1 dead",
    });
    expect(bar).toBeInTheDocument();

    // One segment per present status, sized by its count (running/failed absent).
    const segments = [...container.querySelectorAll<HTMLElement>(".composition-segment")];
    expect(segments.map((s) => s.style.flexGrow)).toEqual(["2", "3", "1"]);
    expect(segments.map((s) => s.className)).toEqual([
      "composition-segment status-pending",
      "composition-segment status-completed",
      "composition-segment status-dead",
    ]);

    const legend = screen.getByRole("list");
    const items = within(legend).getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent("pending 2");
  });

  it("renders nothing when every count is zero", () => {
    const { container } = render(
      <QueueComposition counts={{ pending: 0, running: 0, completed: 0, failed: 0, dead: 0 }} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
