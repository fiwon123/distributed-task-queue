import { describe, expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import About from "./About";

describe("About", () => {
  test("renders key concepts sections", () => {
    render(<About />);
    expect(screen.getByRole("heading", { name: /concepts & how it works/i })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /why distributed task queues/i })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /core concepts/i })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /job lifecycle/i })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /key characteristics/i })).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: /what this project deliberately is/i }),
    ).toBeTruthy();
  });

  test("includes FOR UPDATE SKIP LOCKED and at-least-once concepts", () => {
    render(<About />);
    expect(screen.getAllByText(/for update skip locked/i).length > 0).toBeTruthy();
    expect(screen.getAllByText(/at-least-once/i).length > 0).toBeTruthy();
    expect(screen.getAllByText(/leases/i).length > 0).toBeTruthy();
    expect(screen.getAllByText(/dead-letter/i).length > 0).toBeTruthy();
  });
});
