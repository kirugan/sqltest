package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	host        *string = flag.String("host", "", "target host or address")
	port        *string = flag.String("port", "0", "target port")
	listen_port *string = flag.String("listen_port", "0", "listen port")
)

func die(format string, v ...interface{}) {
	os.Stderr.WriteString(fmt.Sprintf(format+"\n", v...))
	os.Exit(1)
}

// Данная функция реализует поток для двунаправленного дампа.
func connection_logger(data chan []byte, conn_n int, local_info, remote_info string) {
	log_name := fmt.Sprintf("log-%s-%04d-%s-%s.log", format_time(time.Now()),
		conn_n, local_info, remote_info)
	logger_loop(data, log_name)
}

// Данная функция реализует двоичный лог.
func binary_logger(data chan []byte, conn_n int, peer string) {
	log_name := fmt.Sprintf("log-binary-%s-%04d-%s.log", format_time(time.Now()),
		conn_n, peer)
	logger_loop(data, log_name)
}

// Данная функция реализует поток логирования. Создает лог-файл и начинает
// принимает сообщения. Каждое сообщение - это кусок данных для помещения
// в лог. Если пришли пустые данные - выходим.
//
func logger_loop(data chan []byte, log_name string) {
	f, err := os.Create(log_name)
	if err != nil {
		die("Unable to create file %s, %v\n", log_name, err)
	}
	defer f.Close()  // Гарантируем закрытие файла в случае падения.
	for {
		b := <-data
		if len(b) == 0 {
			break
		}
		f.Write(b)
		f.Sync()       // На всякий случай flush'имся.
	}
}

func format_time(t time.Time) string {
	return t.Format("2006.01.02-15.04.05")
}

func printable_addr(a net.Addr) string {
	return strings.Replace(a.String(), ":", "-", -1)
}

// Структура, в которой передаются параметры соединения. Объединено, чтобы
// не таскать много параметров.
type Channel struct {
	from, to net.Conn
	logger, binary_logger chan []byte
	ack chan bool
}

// Функция, "качающая" данные из одного сокета и передающая их в другой.
// Попутно ведется логирование.
func pass_through(c *Channel) {
	from_peer := printable_addr(c.from.LocalAddr())
	to_peer := printable_addr(c.to.LocalAddr())

	b := make([]byte, 10240)
	offset := 0
	packet_n := 0
	for {
		n, err := c.from.Read(b)
		if err != nil {
			c.logger <- []byte(fmt.Sprintf("Disconnected from %s\n", from_peer))
			break
		}
		if n > 0 {
			// Если что-то пришло, то логируем и пересылаем на выход.
			c.logger <- []byte(fmt.Sprintf("Received (#%d, %08X) %d bytes from %s\n",
				packet_n, offset, n, from_peer))
			// Это все, что нужно для преобразования в hex-дамп. Удобно, не так ли?
			c.logger <- []byte(hex.Dump(b[:n]))
			c.binary_logger <- b[:n]
			c.to.Write(b[:n])
			c.logger <- []byte(fmt.Sprintf("Sent (#%d) to %s\n", packet_n, to_peer))
			offset += n
			packet_n += 1
		}
	}
	c.from.Close()
	c.to.Close()
	c.ack <- true   // Посылаем сообщение в главный поток, что мы закончили.
}

// Данная функция обслуживает соединение. Запускает необходимые потоки и ждет
// их завершения.
func process_connection(local net.Conn, conn_n int, target string) {
	// Соединяемся к удаленном сокету, куда будем пересылать данные.
	remote, err := net.Dial("tcp", target)
	if err != nil {
		fmt.Printf("Unable to connect to %s, %v\n", target, err)
	}

	local_info := printable_addr(remote.LocalAddr())
	remote_info := printable_addr(remote.RemoteAddr())

	// Засекаем начальное время.
	started := time.Now()

	// Создаем каналы для обмена с логгерами.
	logger := make(chan []byte)
	from_logger := make(chan []byte)
	to_logger := make(chan []byte)

	// Канал для получения подтверждений от качающих потоков.
	ack := make(chan bool)

	// Запускаем логгеры.
	go connection_logger(logger, conn_n, local_info, remote_info)
	go binary_logger(from_logger, conn_n, local_info)
	go binary_logger(to_logger, conn_n, remote_info)

	logger <- []byte(fmt.Sprintf("Connected to %s at %s\n", target,
		format_time(started)))

	// Запускаем качающие потоки.
	go pass_through(&Channel{remote, local, logger, to_logger, ack})
	go pass_through(&Channel{local, remote, logger, from_logger, ack})

	// Ждем подтверждения об их завершении.
	<-ack
	<-ack

	// Вычисляем длительность соединения.
	finished := time.Now()
	duration := finished.Sub(started)
	logger <- []byte(fmt.Sprintf("Finished at %s, duration %s\n",
		format_time(started), duration.String()))

	// Посылаем логгерам команды закругляться. Мы тут не ждем от них
	// подтверждения, так как они и так завершатся рано или поздно, а они нам
	// более не нужны.
	logger <- []byte{}
	from_logger <- []byte{}
	to_logger <- []byte{}
}

func main() {
	// Просим Go использовать все имеющиеся в системе процессоры.
	runtime.GOMAXPROCS(runtime.NumCPU())
	// Разбираем командную строку (несложно, не правда ли?)
	flag.Parse()
	if flag.NFlag() != 3 {
		fmt.Printf("usage: gotcpspy -host target_host -port target_port -listen_post=local_port\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	target := net.JoinHostPort(*host, *port)
	fmt.Printf("Start listening on port %s and forwarding data to %s\n",
		*listen_port, target)

	ln, err := net.Listen("tcp", ":"+*listen_port)
	if err != nil {
		fmt.Printf("Unable to start listener, %v\n", err)
		os.Exit(1)
	}
	conn_n := 1
	for {
		// Ждем новых соединений.
		if conn, err := ln.Accept(); err == nil {
			// Запускаем поток обработки соединения.
			go process_connection(conn, conn_n, target)
			conn_n += 1
		} else {
			fmt.Printf("Accept failed, %v\n", err)
		}
	}
}