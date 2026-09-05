import { motion } from "framer-motion";
import { LoaderCircle } from "lucide-react";

export function Spinner({ size = 16 }: { size?: number }) {
  return (
    <motion.span className="spinner" animate={{ rotate: 360 }} transition={{ repeat: Infinity, duration: 0.9, ease: "linear" }}>
      <LoaderCircle size={size} />
    </motion.span>
  );
}
