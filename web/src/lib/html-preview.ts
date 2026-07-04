/**
 * Shared helpers for rendering self-contained HTML in a sandboxed iframe.
 *
 * Used by the chat `HtmlPreview` renderer (fenced ```html blocks). It renders
 * via `<iframe sandbox="allow-scripts" srcDoc={buildSrcdoc(...)}>` and relies on
 * the injected resize script to report content height back to the parent.
 */

export const HTML_PREVIEW_HEIGHT_MSG = "dexiask-html-resize";

const resizeScript = `<script>
(function(){
  function report(){
    var h=document.documentElement.scrollHeight||document.body.scrollHeight;
    window.parent.postMessage({type:"${HTML_PREVIEW_HEIGHT_MSG}",height:h},"*");
  }
  if(document.readyState==="complete")report();
  else window.addEventListener("load",report);
  if(typeof ResizeObserver!=="undefined")
    new ResizeObserver(report).observe(document.documentElement);
})();
</script>`;

/**
 * Wrap arbitrary HTML into a full document with the resize script injected.
 * A full `<!doctype html>` / `<html>` document is used as-is (with the script
 * spliced before `</body>`); a fragment is wrapped in a minimal shell.
 */
export function buildSrcdoc(code: string): string {
  const isFullDoc = /^\s*<!doctype\s+html|^\s*<html/i.test(code.trimStart());
  if (isFullDoc) {
    return (
      code.replace(/(<\/body\s*>)/i, `${resizeScript}$1`) || code + resizeScript
    );
  }
  return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>html,body{margin:0;padding:8px;box-sizing:border-box;font-family:system-ui,sans-serif}</style>
</head>
<body>
${code}
${resizeScript}
</body>
</html>`;
}
