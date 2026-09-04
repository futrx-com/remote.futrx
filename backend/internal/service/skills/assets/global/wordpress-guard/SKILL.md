---
name: wordpress-guard
description: "Safe-coding rules for WordPress and WooCommerce - escaping and sanitization, nonces plus capability checks, $wpdb->prepare, core APIs before raw SQL or cURL, HPOS-safe order access through wc_get_order, and translation-ready strings. Use when writing or reviewing plugins, themes, blocks, shortcodes, AJAX or REST handlers, meta boxes, and WooCommerce order, product or checkout logic."
---

# WordPress guard

Apply these rules when writing WordPress or WooCommerce code, and check them
one by one when reviewing it. Every rule below is a place real plugins break
sites or leak data.

## Escaping and sanitization

The rule is **sanitize on input, escape on output, always late, always in the
right context**.

- Input: `sanitize_text_field()`, `sanitize_email()`, `sanitize_key()`,
  `absint()`, `wp_kses_post()` for rich text. Never store raw `$_POST`.
- Output: `esc_html()` for text, `esc_attr()` for attributes, `esc_url()` for
  links, `esc_textarea()`, `wp_kses_post()` for allowed markup, and
  `wp_json_encode()` for data handed to JavaScript.
- Escape at the moment of echo, not when the value is assigned. A value that
  was escaped three functions earlier is not reliably safe here.
- `esc_html_e()` / `esc_html__()` when the string is also translated.
- Never `echo` an unescaped variable, ever — including admin screens, error
  messages, and values that "come from the database", because the database is
  full of user input.

## Nonces and capability checks

These are two different controls and you need both.

- A nonce (`wp_nonce_field()` + `check_admin_referer()` /
  `wp_verify_nonce()`, or `check_ajax_referer()`) proves the request came from
  your form. It proves nothing about permission.
- A capability check (`current_user_can( 'edit_post', $post_id )`) proves the
  user is allowed to do this to *this object*. Prefer the object-aware form
  over a bare `manage_options`.
- Every `admin_post_*`, `wp_ajax_*`, form handler, and REST route needs both.
- `wp_ajax_nopriv_*` is unauthenticated by definition — treat every input as
  hostile and re-check what an anonymous visitor may do.
- REST routes must set a real `permission_callback`. `__return_true` on a
  route that reads or writes anything private is a vulnerability.

## Database access

- Use the API before raw SQL: `WP_Query`, `get_posts()`, `get_option()`,
  `update_post_meta()`, `wp_insert_post()`, `WP_User_Query`, `WP_Term_Query`.
- When raw SQL is unavoidable, always `$wpdb->prepare()`:
  `$wpdb->get_results( $wpdb->prepare( "SELECT * FROM {$wpdb->posts} WHERE post_author = %d AND post_type = %s", $author_id, $type ) );`
- `%s`, `%d`, `%f` only. Placeholders cannot be quoted by you, and cannot
  parameterize table or column names — allowlist those instead.
- Never interpolate a variable into the query string, not even an integer you
  "know" is safe.
- Use `{$wpdb->prefix}` / `$wpdb->posts`; never hardcode `wp_`.
- `LIKE` needs `$wpdb->esc_like()` before it goes into `prepare()`.

## Use core APIs first

- HTTP: `wp_remote_get()` / `wp_remote_post()`, never raw cURL or
  `file_get_contents()` on a URL.
- Filesystem: `WP_Filesystem`, `wp_upload_dir()`, `wp_mkdir_p()`.
- Scripts and styles: `wp_enqueue_script()` / `wp_enqueue_style()` with a
  version and dependencies. Never print `<script src>` by hand.
- Data to JS: `wp_localize_script()` or `wp_add_inline_script()`.
- Cron: `wp_schedule_event()`, and clean up on deactivation.
- Transients / object cache for expensive lookups; never cache per-user data
  in a site-wide transient.
- Dates: `current_time()`, `wp_date()`, and site timezone helpers — not
  `date()` on a raw timestamp.

## Queries and performance

- Never `posts_per_page => -1` on a user-facing query.
- Set `no_found_rows => true` when you do not paginate, and
  `update_post_meta_cache` / `update_post_term_cache` to `false` when you do
  not need that data.
- Avoid `meta_query` as the primary filter on large tables; use a taxonomy or
  a custom table when the data is a real index.
- No queries inside a template loop. Batch and prime caches instead.
- Do not call `wp_remote_*` on a front-end page load without a cache and a
  short timeout.

## WooCommerce

- **HPOS**: read and write orders only through the CRUD API. Use
  `wc_get_order( $order_id )` and `$order->get_*()` / `$order->set_*()` /
  `$order->save()`. Do **not** use `get_post_meta( $order_id, ... )`,
  `WP_Query` over `shop_order`, or direct `wp_posts` / `wp_postmeta` SQL for
  orders — those break on High-Performance Order Storage.
- Order queries go through `wc_get_orders()` / `WC_Order_Query`.
- Declare compatibility in your plugin bootstrap:
  `FeaturesUtil::declare_compatibility( 'custom_order_tables', __FILE__, true );`
- Products: `wc_get_product()` and the product CRUD, not raw meta.
- Custom order data: `$order->update_meta_data()` + `$order->save()`.
- Money: use `wc_format_decimal()`, `wc_price()` for display, and the store's
  decimal precision. Never build totals with float arithmetic and never trust
  a price, quantity, or discount that arrived from the client — recompute
  server-side at checkout.
- Stock changes go through `wc_update_product_stock()` / the product CRUD so
  the stock hooks and HPOS sync fire.
- Extend through hooks (`woocommerce_*` actions and filters) before you
  override a template; an overridden template silently rots on every Woo
  release.
- Checkout fields must be validated server-side in
  `woocommerce_after_checkout_validation` (or the Store API equivalent), not
  only in JavaScript.

## Internationalization

- Wrap every user-facing string: `__()`, `_e()`, `esc_html__()`,
  `esc_html_e()`, `_n()` for plurals, `_x()` when context disambiguates.
- The text domain is a plain string literal matching the plugin slug — never a
  variable or a constant.
- Use `printf()` with placeholders and a translator comment rather than
  concatenating translated fragments:
  `/* translators: %s: order number */`
- Load the text domain on `init` (or rely on core's automatic loading for
  wordpress.org-hosted plugins).

## Structure and hygiene

- Prefix every global function, class, constant, option, and hook with your
  plugin slug, or namespace them.
- Guard direct access at the top of every PHP file:
  `if ( ! defined( 'ABSPATH' ) ) { exit; }`
- Never modify core, and never edit a parent theme — use a child theme or
  hooks.
- Uninstall cleans up its own options, meta, tables, and cron events.
- Follow WordPress Coding Standards (WPCS/PHPCS) and keep the plugin free of
  `error_log()` debris and `var_dump()`.

## Review output

Report findings as **blocker / major / minor** with file, line, the rule that
was broken, and the corrected snippet. Missing capability checks, missing
`prepare()`, unescaped output, and direct order-meta access on WooCommerce are
always blockers.
