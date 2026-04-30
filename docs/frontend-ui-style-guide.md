# XiaoAiAgent Frontend UI Style Guide

This guide defines the default visual direction for XiaoAiAgent frontend pages.

It should be treated as the baseline style constraint for future Dashboard, Settings, Logs, and other frontend page updates unless the user explicitly asks for a redesign.

The approved Dashboard reference image provided in product discussion is the default visual benchmark for this guide.

## 1. Overall Style Positioning

XiaoAiAgent frontend pages should follow this default style direction:

- modern SaaS dashboard
- cute technology feel
- bright and clean
- rounded card-based layout
- light robot mascot decoration
- blue / purple / mint as the main color direction
- professional but not cold
- Chinese-first interface
- moderate information density

This direction applies to:

- Dashboard
- Settings
- Logs
- future management pages

Future frontend changes should extend this language instead of introducing a conflicting visual system.

## 2. Visual Baseline

The current approved Dashboard reference establishes these baseline characteristics:

- fixed left navigation
- large title area with summary metrics
- clear multi-card content layout
- white or near-white cards over a soft bright background
- soft shadows instead of heavy borders
- light mascot and decorative technology elements
- readable Chinese labels with clear hierarchy

When implementing new pages, use this baseline as the first reference before inventing new layout or styling patterns.

## 3. Page Layout Constraints

Dashboard-style pages should follow these layout rules:

- use a fixed or visually stable left navigation bar
- keep a top title and status summary area
- use a multi-card layout for the main content area
- split information into clear functional regions
- maintain comfortable spacing between cards
- avoid heavy border boxes
- prefer soft shadows and subtle light borders
- keep the full page bright, light, and clean

Typical Dashboard homepage structure:

- left navigation: Logo, Dashboard, Settings, Logs, user info
- top area: page title, service status, task metrics, refresh status
- left main card: task list
- center main card: task details, progress, summary
- right card stack: event flow, Artifacts, Claude record, recent conversation

Settings and Logs pages should preserve the same shell and card rhythm even when the content differs.

## 4. Color Constraints

Recommended color direction:

- primary color: blue
- secondary colors: purple and mint
- success state: green
- failure state: red, but softened
- canceled / neutral state: gray or soft purple-gray
- page background: light blue-white, light gray-white, or subtle soft gradient
- cards: white or near-white
- decoration: low-saturation pastel colors

Avoid:

- large dark backgrounds
- neon colors
- heavy black borders
- cold enterprise-gray admin styling
- overly decorative visuals that reduce readability

## 5. Component Style Constraints

Buttons, cards, inputs, selects, tabs, tables, and status labels should follow these rules:

- large rounded corners
- light shadow treatment
- thin borders or visually subtle borders
- soft hover states
- pill-style status badges
- linear or light semi-skeuomorphic icon style
- lists and tables should remain clean and well-separated

Common status color expectations:

- running: blue
- completed: green
- failed: red
- canceled: gray or purple-gray
- service healthy: green

Interactive controls should feel polished and friendly, not like raw browser defaults or engineering-only console widgets.

## 6. Illustration and Decoration Constraints

Allowed decorative elements:

- cute AI robot mascot
- small stars
- clouds
- soft dots
- small technology-themed icons
- low-contrast gradient blocks

Rules:

- decoration must not cover content
- decoration must not reduce readability
- mascot should feel cute, friendly, and lightly technological
- illustration is supporting material, not the main content
- product information always has priority over decoration

Do not remove mascot-style brand recognition without an explicit request.

## 7. Typography and Layout Density

- Chinese UI comes first
- headings should be clear and strongly hierarchical
- body text should stay readable
- time, ID, and status information should align clearly
- avoid font sizes that are too small for a management page
- avoid cramped card content
- keep card copy concise and scannable

The dashboard should feel informative, not crowded.

## 8. Rules for Future Frontend Changes

Any future frontend UI change must follow this order:

1. read this style guide first
2. preserve consistency across Dashboard, Settings, and Logs
3. reuse existing shell, card, status badge, spacing, and color patterns
4. extend the existing design language instead of replacing it

When adding new pages or sections:

- reuse the existing overall page shell
- reuse rounded cards and soft status badges
- stay within the blue / purple / mint direction
- keep Chinese labels readable and product-oriented
- keep the cute technology branding visible but restrained

Unless the user explicitly asks for a redesign:

- do not radically change the overall visual language
- do not replace the shell with a totally different admin framework style
- do not turn the interface into a raw engineering control panel

## 9. Things That Should Not Be Done

Explicitly disallowed by default:

- turning the UI into a dark hacker style
- turning the UI into a traditional Bootstrap admin style
- turning the UI into a minimal no-brand style
- using too many strong gradients
- using busy backgrounds that reduce readability
- removing mascot / cute technology decoration without request
- changing the main layout structure without request

## 10. Implementation Notes

This guide is about visual and interaction style constraints.

It does not require:

- changing backend APIs
- rewriting page business logic
- introducing runtime dependency on the reference image

If a future change needs a major redesign, that redesign should first update this guide or explicitly state why it is intentionally diverging.
