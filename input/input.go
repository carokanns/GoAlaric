package input

import (
	"bufio"
	"io"
	"os"
)

/*
   class Input : public util::Waitable {

      bool volatile p_has_input;
      bool p_eof;
      std::string p_line;

   public:

      Input() {
         p_has_input = false;
         p_eof = false;
      }

      bool has_input() const {
         return p_has_input;
      }

      bool get_line(std::string & line) {

         lock();

         while (!p_has_input) {
            wait();
         }

         bool line_ok = !p_eof;
         if (line_ok) line = p_line;

         p_has_input = false;
         signal();

         unlock();

         return line_ok;
      }

      void set_eof() {

         lock();

         while (p_has_input) {
            wait();
         }

         p_eof = true;

         p_has_input = true;
         signal();

         unlock();
      }

      void set_line(std::string & line) {

         lock();

         while (p_has_input) {
            wait();
         }

         p_line = line;

         p_has_input = true;
         signal();

         unlock();
      }

   };

   Input input;
   std::thread thread;
*/
var reader = bufio.NewReader(os.Stdin)

// GetInput gets the next input from stdin (the GUI)
func GetInput(line chan<- string) {
	//reader = bufio.NewReader(os.Stdin)
	for {
		text, err := reader.ReadString('\n')
		if err != io.EOF && len(text) > 0 {
			line <- text
		}
	}
}
