// Sample source: glossa-* keys we expect the scanner to find.

export function Cart() {
  return (
    <button>
      <glossa-text key="cart.checkout">Approve plan</glossa-text>
    </button>
  );
}

export function Greeting() {
  return (
    <glossa-rich key="athlete.greeting" vars={'{"name":"Sophia"}'}>
      Hi, ${"{name}"}!
    </glossa-rich>
  );
}
