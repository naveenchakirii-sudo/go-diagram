import { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { actions } from '../diagramSlice';
import Struct from './Struct';
import { basename, edgePath, fieldKey, structKey } from '../refs';

const MIN_ZOOM = 0.25;
const MAX_ZOOM = 2;

export default function UMLDiagram() {
  const dispatch = useDispatch();
  const packages = useSelector((s) => s.diagram.packages);
  const edges = useSelector((s) => s.diagram.edges);

  const [view, setView] = useState({ x: 0, y: 0, zoom: 1 });
  const [selection, setSelection] = useState({ pkg: null, file: null });
  const [paths, setPaths] = useState([]);
  const [dragging, setDragging] = useState(false);
  const [size, setSize] = useState({ width: 0, height: 0 });
  const drag = useRef(null);
  const diagram = useRef(null);
  const nodes = useRef(new Map());

  const registerNode = useCallback((key, el) => {
    if (el) nodes.current.set(key, el);
    else nodes.current.delete(key);
  }, []);

  // Measure rendered structs and draw edges between them. Runs after every
  // data change and whenever the laid-out content changes size.
  const layoutEdges = useCallback(() => {
    const root = diagram.current;
    if (!root) return;
    const origin = root.getBoundingClientRect();
    const zoom = view.zoom;
    const rel = (el) => {
      const r = el.getBoundingClientRect();
      return {
        left: (r.left - origin.left) / zoom,
        right: (r.right - origin.left) / zoom,
        top: (r.top - origin.top) / zoom,
        bottom: (r.bottom - origin.top) / zoom,
        height: r.height / zoom,
      };
    };
    const next = [];
    edges.forEach((edge, i) => {
      const { from, to } = edge;
      const fromEl =
        nodes.current.get(fieldKey(from.packageName, from.fileName, from.structName, from.fieldTypeName)) ||
        nodes.current.get(structKey(from.packageName, from.fileName, from.structName));
      const toEl = nodes.current.get(structKey(to.packageName, to.fileName, to.structName));
      if (!fromEl || !toEl) return;
      const d = edgePath(rel(fromEl), rel(toEl));
      if (d) next.push({ id: i, d, label: `${from.structName}.${from.fieldTypeName} → ${to.structName}` });
    });
    setPaths(next);
    setSize({ width: root.scrollWidth + 200, height: root.scrollHeight + 200 });
  }, [edges, view.zoom]);

  useLayoutEffect(() => {
    layoutEdges();
    const observer = new ResizeObserver(() => layoutEdges());
    if (diagram.current) observer.observe(diagram.current.querySelector('.packages'));
    return () => observer.disconnect();
  }, [layoutEdges, packages]);

  const onPointerDown = (e) => {
    if (e.button !== 0 || e.target.closest('button, input')) return;
    drag.current = { x: e.clientX - view.x, y: e.clientY - view.y };
    setDragging(true);
  };
  const onPointerMove = (e) => {
    if (!drag.current) return;
    setView((v) => ({ ...v, x: e.clientX - drag.current.x, y: e.clientY - drag.current.y }));
  };
  const onPointerUp = () => {
    drag.current = null;
    setDragging(false);
  };
  const onWheel = (e) => {
    if (!e.ctrlKey && !e.metaKey) {
      setView((v) => ({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }));
      return;
    }
    setView((v) => {
      const zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, v.zoom * (e.deltaY < 0 ? 1.1 : 1 / 1.1)));
      // Zoom around the cursor.
      const k = zoom / v.zoom;
      return { zoom, x: e.clientX - (e.clientX - v.x) * k, y: e.clientY - (e.clientY - v.y) * k };
    });
  };
  const zoomBy = (factor) =>
    setView((v) => ({ ...v, zoom: Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, v.zoom * factor)) }));

  const onBackgroundClick = (e) => {
    if (!e.target.closest('.package')) setSelection({ pkg: null, file: null });
  };

  return (
    <div
      className={`UMLDiagram${dragging ? ' dragging' : ''}`}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onPointerLeave={onPointerUp}
      onWheel={onWheel}
      onClick={onBackgroundClick}
    >
      <div
        className="diagram"
        ref={diagram}
        style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.zoom})` }}
      >
        <section className="packages">
          {packages.map((pkg) => (
            <section
              key={pkg.name}
              className={`package${selection.pkg === pkg.name && !selection.file ? ' selected' : ''}`}
              onClick={(e) => {
                if (!e.target.closest('.file')) setSelection({ pkg: pkg.name, file: null });
              }}
            >
              <h3 className="title">{pkg.name}</h3>
              {pkg.files.map((file) => (
                <div
                  key={file.name}
                  className={`file${selection.file === file.name ? ' selected' : ''}`}
                  onClick={(e) => {
                    if (!e.target.closest('.Struct')) setSelection({ pkg: pkg.name, file: file.name });
                  }}
                >
                  <h3 className="title" title={file.name}>
                    {basename(file.name)}
                  </h3>
                  <button
                    type="button"
                    className="Button addStruct"
                    title="Add struct"
                    onClick={() => dispatch(actions.addStruct({ package: pkg.name, file: file.name }))}
                  >
                    +
                  </button>
                  {file.structs.length === 0 && <p className="empty">no structs</p>}
                  {file.structs.map((struct) => (
                    <Struct
                      key={struct.name}
                      pkg={pkg.name}
                      file={file.name}
                      struct={struct}
                      registerNode={registerNode}
                    />
                  ))}
                </div>
              ))}
            </section>
          ))}
        </section>
        <svg className="edges" width={size.width} height={size.height} aria-hidden="true">
          <defs>
            <marker id="head" markerWidth="10" markerHeight="10" refX="6" refY="3" orient="auto" markerUnits="strokeWidth">
              <path d="M0,0 L0,6 L7,3 z" fill="white" opacity="0.7" />
            </marker>
          </defs>
          {paths.map((p) => (
            <path key={p.id} className="edgeline" d={p.d} markerEnd="url(#head)">
              <title>{p.label}</title>
            </path>
          ))}
        </svg>
      </div>
      <div className="zoom-controls">
        <button type="button" className="Button" onClick={() => zoomBy(1.2)} title="Zoom in">
          +
        </button>
        <button type="button" className="Button" onClick={() => zoomBy(1 / 1.2)} title="Zoom out">
          −
        </button>
        <button type="button" className="Button" onClick={() => setView({ x: 0, y: 0, zoom: 1 })} title="Reset view">
          ⟲
        </button>
      </div>
    </div>
  );
}
