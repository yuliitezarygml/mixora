export default function Icon({ name, size = 24, ...props }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      aria-hidden="true"
      {...props}
    >
      <use href={`/assets/icons/sprite.svg#${name}`} />
    </svg>
  );
}
