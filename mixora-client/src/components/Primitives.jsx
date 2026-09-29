import { useEffect, useRef } from "react";
import { Link } from "react-router-dom";
import Icon from "./Icon.jsx";
export function IconButton({ icon, label, active = false, ...props }) {
  return (
    <button
      className={`icon-button ${active ? "active" : ""}`}
      aria-label={label}
      title={label}
      {...props}
    >
      <Icon name={icon} />
    </button>
  );
}
export function Empty({
  icon = "note_xl",
  title = "Здесь пока пусто",
  text,
  action,
}) {
  return (
    <div className="empty">
      <Icon name={icon} size={64} />
      <h2>{title}</h2>
      {text && <p>{text}</p>}
      {action}
    </div>
  );
}
export function Tabs({ items }) {
  return (
    <div className="tabs TabCarousel_root__8DoRy">
      {items.map(({ label, to, active }) => (
        <Link
          className={`Tab_root__LUukY Tab_tab_size_m__c7tVg ${active ? "selected" : ""}`}
          key={to}
          to={to}
        >
          {label}
        </Link>
      ))}
    </div>
  );
}
export function Section({ title, to, children, action }) {
  return (
    <section className="section">
      <div className="section-heading BlockHeader_root__j3mbg">
        <h2>
          {to ? (
            <Link to={to}>
              {title}
              <Icon name="arrowRight_xs" />
            </Link>
          ) : (
            title
          )}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}
export function Modal({ title, onClose, children }) {
  const ref = useRef(null);
  useEffect(() => {
    const node = ref.current;
    const previous = document.activeElement;
    node.showModal();
    return () => {
      node.close();
      previous?.focus?.();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className="modal"
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <header>
        <h2>{title}</h2>
        <IconButton icon="close_xs" label="Закрыть" onClick={onClose} />
      </header>
      {children}
    </dialog>
  );
}
export function Cover({ track, large = false }) {
  return track?.artwork ? (
    <img
      className={`cover ${large ? "large" : ""}`}
      src={track.artwork.replace("-large.", "-t500x500.")}
      alt=""
      loading="lazy"
      onError={(e) => {
        e.currentTarget.style.visibility = "hidden";
      }}
    />
  ) : (
    <div className={`cover fallback ${large ? "large" : ""}`}>
      <Icon name="note_xl" size={large ? 80 : 24} />
    </div>
  );
}
