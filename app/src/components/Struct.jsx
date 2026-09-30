import { memo } from 'react';
import { useDispatch } from 'react-redux';
import { actions } from '../diagramSlice';
import AutosizeInput from './AutosizeInput';
import { structKey } from '../refs';

const TYPE_CLASSES = new Set(['string', 'int', 'bool']);
const IDENT = /^[A-Za-z_][A-Za-z0-9_]*$/;

function typeClass(literal) {
  return TYPE_CLASSES.has(literal) ? literal : 'other';
}

function Struct({ pkg, file, struct, registerNode }) {
  const dispatch = useDispatch();
  const ref = { package: pkg, file, name: struct.name };
  const key = structKey(pkg, file, struct.name);

  const commitName = (newName) => {
    if (IDENT.test(newName)) dispatch(actions.renameStruct({ ...ref, newName }));
  };

  return (
    <div className="Struct" ref={(el) => registerNode(key, el)}>
      <header className="header">
        <button
          type="button"
          className="class icon"
          title="Add field"
          onClick={() => dispatch(actions.addField(ref))}
        >
          <span className="c">c</span>
          <span className="p">+</span>
        </button>
        <AutosizeInput className="name" value={struct.name} minWidth={100} onCommit={commitName} aria-label="Struct name" />
        {struct.typeParams && <span className="typeparams">{struct.typeParams}</span>}
        <button
          type="button"
          className="delete icon"
          title="Delete struct"
          onClick={() => {
            if (window.confirm(`Delete struct ${struct.name}? This edits the source file.`)) {
              dispatch(actions.deleteStruct(ref));
            }
          }}
        >
          x
        </button>
      </header>
      <ol className="fields">
        {struct.fields.map((field, index) => (
          <li
            // Field names can repeat while editing, so index is the stable key here.
            // eslint-disable-next-line react/no-array-index-key
            key={index}
            className={`field${field.embedded ? ' embedded' : ''}`}
            ref={(el) => registerNode(`${key}#${field.name}`, el)}
            title={field.tag || undefined}
          >
            <span className="left">
              <button
                type="button"
                className="field icon"
                title="Remove field"
                onClick={() => dispatch(actions.removeField({ ...ref, index }))}
              >
                <span className="f">f</span>
                <span className="x">x</span>
              </button>
              {field.embedded ? (
                <span className="name embedded-label" title="Embedded field">
                  (embedded)
                </span>
              ) : (
                <AutosizeInput
                  className="name"
                  value={field.name}
                  minWidth={60}
                  aria-label="Field name"
                  onCommit={(newName) => {
                    if (IDENT.test(newName)) dispatch(actions.renameField({ ...ref, index, newName }));
                  }}
                />
              )}
            </span>
            <span className="right">
              <AutosizeInput
                className={`type ${typeClass(field.type.literal)}`}
                value={field.type.literal}
                minWidth={40}
                aria-label="Field type"
                onCommit={(newType) => dispatch(actions.changeFieldType({ ...ref, index, newType }))}
              />
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

export default memo(Struct);
