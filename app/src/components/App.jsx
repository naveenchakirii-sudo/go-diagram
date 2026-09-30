import { useDispatch, useSelector } from 'react-redux';
import { actions } from '../diagramSlice';
import UMLDiagram from './UMLDiagram';

const STATUS_TEXT = {
  connecting: 'Connecting to go-diagram server…',
  disconnected: 'Disconnected — retrying…',
};

export default function App() {
  const dispatch = useDispatch();
  const status = useSelector((s) => s.diagram.status);
  const error = useSelector((s) => s.diagram.error);
  const empty = useSelector((s) => s.diagram.packages.length === 0);

  return (
    <div className="App HomePage">
      <UMLDiagram />
      {STATUS_TEXT[status] && <div className="banner status">{STATUS_TEXT[status]}</div>}
      {status === 'connected' && empty && (
        <div className="banner status">No structs found (packages named main are skipped).</div>
      )}
      {error && (
        <div className="banner error" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => dispatch(actions.dismissError())} aria-label="Dismiss">
            ×
          </button>
        </div>
      )}
      <div className="help">drag to pan · ctrl+scroll to zoom · edits save to your .go files</div>
    </div>
  );
}
