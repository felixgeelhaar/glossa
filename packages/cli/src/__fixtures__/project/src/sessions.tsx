export function Sessions({ count }: { count: number }) {
  return (
    <p>
      <glossa-plural key="athlete.session_count" count={count}>
        no sessions
      </glossa-plural>
      {" — "}
      <glossa-select key="user.gender" value="female">
        they
      </glossa-select>
    </p>
  );
}

// Duplicate of an earlier key — scanner must dedupe by name.
export function CheckoutAgain() {
  return <glossa-text key="cart.checkout">Re-approve</glossa-text>;
}
