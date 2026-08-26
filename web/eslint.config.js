import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {ignores: ["dist", "coverage", "node_modules", "android", "test-output", "e2e", "playwright.config.ts"]},
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    plugins: {"react-hooks": reactHooks},
    rules: {
      ...reactHooks.configs.recommended.rules,
      // The SPA intentionally keeps canonical English literals inline; tests
      // and the DOM translator rely on them.
      "@typescript-eslint/no-explicit-any": "off",
      // "_" marks deliberately unused bindings (catch-all handlers, reserved
      // destructured slots kept for their setters).
      "@typescript-eslint/no-unused-vars": ["error", {argsIgnorePattern:"^_", varsIgnorePattern:"^_", caughtErrors:"none"}]
    }
  },
  {languageOptions: {globals: {...globals.browser}}}
);
