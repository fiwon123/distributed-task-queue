import { MoonIcon, SunIcon } from "./icons";
import { useTheme } from "./theme";

// ThemeToggle is the moon/sun button in the header. The icon shows what a click
// will switch *to* (a sun while dark), which is why the aria-label is phrased
// from the action rather than the current state.
export default function ThemeToggle() {
  const { theme, toggle } = useTheme();
  const target = theme === "dark" ? "light" : "dark";
  const label = `Switch to ${target} theme`;

  return (
    <button
      type="button"
      className="icon-button"
      onClick={toggle}
      aria-label={label}
      title={label}
      data-testid="theme-toggle"
    >
      {theme === "dark" ? <SunIcon /> : <MoonIcon />}
    </button>
  );
}
