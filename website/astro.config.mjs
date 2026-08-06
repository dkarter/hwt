import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

export default defineConfig({
  site: "https://hwt.doriankarter.com",
  prefetch: false,
  integrations: [
    starlight({
      title: "hwt",
      description: "Deterministic Herdr worktree orchestration.",
      favicon: "/favicon.svg",
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/dkarter/hwt",
        },
      ],
      customCss: ["./src/styles/starlight.css"],
      components: {
        Search: "./src/components/Search.astro",
        SiteTitle: "./src/components/SiteTitle.astro",
      },
      editLink: {
        baseUrl: "https://github.com/dkarter/hwt/edit/main/website/",
      },
      lastUpdated: true,
      disable404Route: true,
      sidebar: [
        {
          label: "Start here",
          items: [
            { label: "Overview", slug: "docs" },
            { label: "Install", slug: "docs/install" },
            { label: "Quick start", slug: "docs/quick-start" },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "Configuration", slug: "docs/configuration" },
            { label: "Copy strategies", slug: "docs/copy-strategies" },
          ],
        },
        {
          label: "Reference",
          items: [
            { label: "CLI overview", slug: "docs/cli-reference" },
            { label: "Create worktree", slug: "docs/cli/create-worktree" },
            { label: "Remove worktree", slug: "docs/cli/remove-worktree" },
            { label: "List worktrees", slug: "docs/cli/list-worktrees" },
          ],
        },
        {
          label: "Misc",
          items: [{ label: "Shell completions", slug: "docs/misc/shell-completions" }],
        },
      ],
    }),
  ],
});
