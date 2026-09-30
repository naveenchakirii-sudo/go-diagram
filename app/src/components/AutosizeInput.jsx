import { useLayoutEffect, useRef, useState } from 'react';

let canvas;
function textWidth(text, el) {
  canvas = canvas || document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  const cs = window.getComputedStyle(el);
  ctx.font = `${cs.fontStyle} ${cs.fontWeight} ${cs.fontSize} ${cs.fontFamily}`;
  const padding = parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight);
  return Math.ceil(ctx.measureText(text || ' ').width + padding + 4);
}

// Text input that grows with its content (replaces the unmaintained
// react-input-autosize). Edits are kept locally and committed on blur or
// Enter; Escape reverts. The draft resets whenever the committed value changes.
export default function AutosizeInput({ value, onCommit, className = '', minWidth = 20, ...rest }) {
  const [draft, setDraft] = useState(value);
  const [width, setWidth] = useState(minWidth);
  const input = useRef(null);

  useLayoutEffect(() => setDraft(value), [value]);
  useLayoutEffect(() => {
    if (input.current) setWidth(Math.max(minWidth, textWidth(draft, input.current)));
  }, [draft, minWidth]);

  const commit = () => {
    const next = draft.trim();
    if (next && next !== value) onCommit(next);
    else setDraft(value);
  };

  return (
    <span className="AutosizeInput">
      <input
        {...rest}
        ref={input}
        className={className}
        value={draft}
        style={{ width }}
        autoComplete="off"
        spellCheck={false}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') input.current.blur();
          if (e.key === 'Escape') {
            setDraft(value);
            requestAnimationFrame(() => input.current && input.current.blur());
          }
        }}
        onPointerDown={(e) => e.stopPropagation()}
      />
    </span>
  );
}
