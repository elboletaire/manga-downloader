// Small form controls shared by the source bar and the options. Each option
// commits on its own: toggles at once, text when it loses focus or on Enter.
// A refused commit (the backend says no) puts the committed value back.

import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { errorMessage } from './format';

interface TextFieldProps {
  label: string;
  value: string;
  onCommit: (value: string) => Promise<void>;
  onError: (message: string) => void;
  placeholder?: string;
  hint?: ReactNode;
  disabled?: boolean;
  mono?: boolean;
  /** Marks the field as changed-but-not-applied */
  flagged?: boolean;
  className?: string;
  /** A control drawn inside the box's right end */
  action?: ReactNode;
}

interface SelectFieldProps {
  label: string;
  value: string;
  options: { value: string; label: string; disabled?: boolean }[];
  /** Called with the picked value; the select only fires on a change */
  onChange: (value: string) => void;
  disabled?: boolean;
  className?: string;
}

/** A native select: keyboard and screen reader behaviour come from the browser */
export function SelectField({ label, value, options, onChange, disabled, className }: SelectFieldProps) {
  const id = useId();

  return (
    <div className={`field ${className ?? ''}`}>
      <label htmlFor={id}>{label}</label>
      <select
        id={id}
        className="input select-input"
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value} disabled={o.disabled}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  );
}

/** A text option. Escape reverts to the committed value. */
export function TextField(props: TextFieldProps) {
  const { label, value, onCommit, onError, placeholder, hint, disabled, mono, flagged, className, action } = props;
  const [draft, setDraft] = useState(value);
  // the draft the last commit sent, until the backend pushes it back: Enter
  // commits and the blur that follows must not send it again
  const sent = useRef<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const id = useId();

  // a pushed value replaces the draft; while typing, pushes carry the old
  // value unchanged, so this doesn't fire and the draft survives
  useEffect(() => {
    setDraft(value);
    sent.current = null;
  }, [value]);

  const commit = () => {
    if (draft === value || draft === sent.current) {
      return;
    }
    sent.current = draft;
    onCommit(draft).catch((e: unknown) => {
      sent.current = null;
      setDraft(value);
      onError(errorMessage(e));
    });
  };

  const input = (
    <input
      ref={inputRef}
      id={id}
      type="text"
      className={`input${mono ? ' mono' : ''}${flagged ? ' flagged' : ''}`}
      value={draft}
      placeholder={placeholder}
      disabled={disabled}
      spellCheck={false}
      autoComplete="off"
      aria-describedby={hint ? `${id}-hint` : undefined}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          commit();
        } else if (e.key === 'Escape') {
          setDraft(value);
        }
      }}
    />
  );

  return (
    <div className={`field ${className ?? ''}`}>
      <label htmlFor={id}>{label}</label>
      {/* the wrapper stays while an action is passed (even when it's false), so the input isn't remounted */}
      {action !== undefined ? (
        <div className="input-wrap">
          {input}
          {/* the action's click focuses the field back, as the button is removed when it resets */}
          {action && (
            <div className="input-action" onClick={() => inputRef.current?.focus()}>
              {action}
            </div>
          )}
        </div>
      ) : (
        input
      )}
      {hint && (
        <p className="hint" id={`${id}-hint`}>
          {hint}
        </p>
      )}
    </div>
  );
}

interface SwitchProps {
  label: string;
  description?: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onChange: (on: boolean) => void;
}

/** An on/off option, drawn as a switch but a real checkbox underneath */
export function Switch({ label, description, checked, disabled, onChange }: SwitchProps) {
  const id = useId();
  return (
    <div className="switch-row">
      <div className="switch-text">
        <label htmlFor={id}>{label}</label>
        {description && (
          <p className="hint" id={`${id}-desc`}>
            {description}
          </p>
        )}
      </div>
      <input
        id={id}
        type="checkbox"
        role="switch"
        className="switch"
        checked={checked}
        disabled={disabled}
        aria-describedby={description ? `${id}-desc` : undefined}
        onChange={(e) => onChange(e.target.checked)}
      />
    </div>
  );
}

interface SegmentedProps<T extends string> {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  disabled?: boolean;
  onChange: (value: T) => void;
}

/** A choice of a few, as radio buttons drawn side by side */
export function Segmented<T extends string>({ label, value, options, disabled, onChange }: SegmentedProps<T>) {
  const name = useId();
  return (
    <fieldset className="field segmented-field" disabled={disabled}>
      <legend>{label}</legend>
      <div className="segmented">
        {options.map((o) => (
          <label key={o.value} className={o.value === value ? 'on' : undefined}>
            <input
              type="radio"
              name={name}
              value={o.value}
              checked={o.value === value}
              onChange={() => onChange(o.value)}
            />
            <span>{o.label}</span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

interface CheckProps {
  label: ReactNode;
  checked: boolean;
  disabled?: boolean;
  onChange: (on: boolean) => void;
}

export function Check({ label, checked, disabled, onChange }: CheckProps) {
  return (
    <label className="check">
      <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
      <span>{label}</span>
    </label>
  );
}
