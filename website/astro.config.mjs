import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import { existsSync, readFileSync } from "node:fs";

const releaseManifest = JSON.parse(
  readFileSync(new URL("../.release-please-manifest.json", import.meta.url), "utf8"),
);
const docsVersion = process.env.HWT_DOCS_VERSION || "dev";
const stableVersion = process.env.HWT_STABLE_VERSION || releaseManifest["."];
const base = process.env.HWT_SITE_BASE || (docsVersion === "dev" ? "/dev" : "/");

process.env.PUBLIC_HWT_STABLE_VERSION = stableVersion;
process.env.PUBLIC_HWT_DOCS_VERSION = docsVersion;
process.env.PUBLIC_HWT_STABLE_ROUTES = process.env.HWT_STABLE_ROUTES || "";

const docsRoot = new URL("./src/content/docs/", import.meta.url);
const doc = (label, slug, file = `${slug.replace(/^docs\//, "")}.md`) =>
  existsSync(new URL(file, docsRoot)) ? { label, slug } : null;
const available = (items) => items.filter(Boolean);

export default defineConfig({
  site: "https://hwt.doriankarter.com",
  base,
  prefetch: false,
  integrations: [
    starlight({
      title: "hwt",
      description: "Frictionless Herdr worktree orchestration.",
      favicon: "/favicon.svg",
      head: [
        {
          tag: "meta",
          attrs: {
            property: "og:image",
            content: "https://hwt.doriankarter.com/og-image.png",
          },
        },
        {
          tag: "meta",
          attrs: { property: "og:image:type", content: "image/png" },
        },
        {
          tag: "meta",
          attrs: { property: "og:image:width", content: "1200" },
        },
        {
          tag: "meta",
          attrs: { property: "og:image:height", content: "630" },
        },
        {
          tag: "meta",
          attrs: {
            property: "og:image:alt",
            content: "hwt creates a ready-to-work Herdr workspace from a Git branch",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "twitter:image",
            content: "https://hwt.doriankarter.com/og-image.png",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "twitter:image:alt",
            content: "hwt creates a ready-to-work Herdr workspace from a Git branch",
          },
        },
      ],
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
        LanguageSelect: "./src/components/VersionSelect.astro",
      },
      editLink: {
        baseUrl: `https://github.com/dkarter/hwt/edit/${
          docsVersion === "dev" ? "main" : `v${stableVersion}`
        }/website/`,
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
          items: available([
            doc("Configuration", "docs/configuration"),
            doc("Herdr plugin", "docs/herdr-plugin"),
            doc("Copy strategies", "docs/copy-strategies", "copy-strategies.mdx"),
          ]),
        },
        {
          label: "Reference",
          items: available([
            doc("CLI overview", "docs/cli-reference"),
            doc("Create worktree", "docs/cli/create-worktree"),
            doc("Remove worktree", "docs/cli/remove-worktree"),
            doc("List worktrees", "docs/cli/list-worktrees"),
            doc("Review pull request", "docs/cli/review-pull-request"),
            doc("Open pull request", "docs/cli/open-pull-request"),
            doc("Open preview", "docs/cli/open-preview-environment"),
            doc("Agent skill", "docs/agent-skill"),
          ]),
        },
        {
          label: "Misc",
          items: [{ label: "Shell completions", slug: "docs/misc/shell-completions" }],
        },
      ],
    }),
  ],
});
